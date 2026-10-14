package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

// Retention of the stats tables (ARCHITECTURE.md 6).
const (
	RawRetention    = 48 * time.Hour
	RollupRetention = 90 * 24 * time.Hour
	rollupBucket    = 5 * time.Minute
	maxPoints       = 20000
)

// Resolution selects the stats table.
type Resolution string

const (
	ResolutionRaw     Resolution = "raw"
	ResolutionFiveMin Resolution = "5m"
)

// StatPoint is one point of a node's time series. Nil means not measured.
type StatPoint struct {
	TS          time.Time
	CPUPct      *float64
	MemUsed     *int64
	MemTotal    *int64
	DiskUsedPct *float64
	NetRxBps    *int64
	NetTxBps    *int64
	CPUTempC    *float64
	GPUUtilPct  *float64
}

// InsertStats stores a sample received at the given time and updates the
// node's last-seen time. Received time, not the agent clock, orders samples.
func (s *Store) InsertStats(ctx context.Context, nodeID string, received time.Time, st proto.Stats) error {
	detail, err := json.Marshal(st)
	if err != nil {
		return err
	}
	var diskMax *float64
	for _, d := range st.Disks {
		if d.Total > 0 {
			pct := float64(d.Used) / float64(d.Total) * 100
			if diskMax == nil || pct > *diskMax {
				diskMax = &pct
			}
		}
	}
	var rx, tx uint64
	for _, n := range st.Net {
		rx += n.RxBps
		tx += n.TxBps
	}
	var gpuMax *float64
	for _, g := range st.GPUs {
		if g.UtilPct != nil && (gpuMax == nil || *g.UtilPct > *gpuMax) {
			gpuMax = g.UtilPct
		}
	}
	onBattery := st.Battery != nil && !st.Battery.OnAC

	batch := &pgx.Batch{}
	batch.Queue(`
		INSERT INTO node_stats_raw (node_id, ts, cpu_pct, load1, mem_used, mem_total, swap_used, disk_used_pct_max,
			net_rx_bps, net_tx_bps, cpu_temp_c, gpu_util_pct, on_battery, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT DO NOTHING`,
		nodeID, received, st.CPU.Pct, st.CPU.Load1, clampInt64(st.Mem.Used), clampInt64(st.Mem.Total), clampInt64(st.Mem.SwapUsed), diskMax,
		clampInt64(rx), clampInt64(tx), st.CPU.TempC, gpuMax, onBattery, detail)
	batch.Queue(`UPDATE nodes SET last_seen_at = $2 WHERE id = $1`, nodeID, received)
	if err := s.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("store: insert stats: %w", err)
	}
	return nil
}

// clampInt64 converts byte counters for bigint columns. Real values are far
// below 2^63; a corrupt agent value must not wrap into a negative number.
func clampInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

// QueryStats returns the series of a node in [from, to).
func (s *Store) QueryStats(ctx context.Context, nodeID string, from, to time.Time, res Resolution) ([]StatPoint, error) {
	var q string
	switch res {
	case ResolutionRaw:
		q = `SELECT ts, cpu_pct, mem_used, mem_total, disk_used_pct_max, net_rx_bps, net_tx_bps, cpu_temp_c, gpu_util_pct
			FROM node_stats_raw WHERE node_id = $1 AND ts >= $2 AND ts < $3 ORDER BY ts LIMIT $4`
	case ResolutionFiveMin:
		q = `SELECT bucket, cpu_pct_avg, mem_used_avg, mem_total, disk_used_pct_max, net_rx_bps_avg, net_tx_bps_avg,
			cpu_temp_c_max, gpu_util_pct_avg
			FROM node_stats_5m WHERE node_id = $1 AND bucket >= $2 AND bucket < $3 ORDER BY bucket LIMIT $4`
	default:
		return nil, fmt.Errorf("store: unknown resolution %q", res)
	}
	rows, err := s.pool.Query(ctx, q, nodeID, from, to, maxPoints)
	if err != nil {
		return nil, fmt.Errorf("store: query stats: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (StatPoint, error) {
		var p StatPoint
		err := r.Scan(&p.TS, &p.CPUPct, &p.MemUsed, &p.MemTotal, &p.DiskUsedPct, &p.NetRxBps, &p.NetTxBps, &p.CPUTempC, &p.GPUUtilPct)
		return p, err
	})
}

// Rollup aggregates completed 5-minute buckets of the last 15 minutes. It is
// idempotent and runs every minute.
func (s *Store) Rollup(ctx context.Context, now time.Time) error {
	end := now.UTC().Truncate(rollupBucket)
	start := end.Add(-3 * rollupBucket)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO node_stats_5m (node_id, bucket, cpu_pct_avg, cpu_pct_max, mem_used_avg, mem_used_max, mem_total,
			disk_used_pct_max, net_rx_bps_avg, net_tx_bps_avg, cpu_temp_c_max, gpu_util_pct_avg)
		SELECT node_id, date_bin('5 minutes', ts, TIMESTAMPTZ '2000-01-01 00:00:00+00'),
			avg(cpu_pct), max(cpu_pct), avg(mem_used)::bigint, max(mem_used), max(mem_total),
			max(disk_used_pct_max), avg(net_rx_bps)::bigint, avg(net_tx_bps)::bigint, max(cpu_temp_c), avg(gpu_util_pct)
		FROM node_stats_raw WHERE ts >= $1 AND ts < $2
		GROUP BY 1, 2
		ON CONFLICT (node_id, bucket) DO UPDATE SET
			cpu_pct_avg = EXCLUDED.cpu_pct_avg, cpu_pct_max = EXCLUDED.cpu_pct_max,
			mem_used_avg = EXCLUDED.mem_used_avg, mem_used_max = EXCLUDED.mem_used_max, mem_total = EXCLUDED.mem_total,
			disk_used_pct_max = EXCLUDED.disk_used_pct_max, net_rx_bps_avg = EXCLUDED.net_rx_bps_avg,
			net_tx_bps_avg = EXCLUDED.net_tx_bps_avg, cpu_temp_c_max = EXCLUDED.cpu_temp_c_max,
			gpu_util_pct_avg = EXCLUDED.gpu_util_pct_avg`, start, end)
	if err != nil {
		return fmt.Errorf("store: rollup: %w", err)
	}
	return nil
}

var partitionedTables = []struct {
	name      string
	retention time.Duration
}{
	{"node_stats_raw", RawRetention},
	{"node_stats_5m", RollupRetention},
}

const partitionSuffix = "_p20060102"

// EnsurePartitions creates the daily partitions from yesterday to two days
// ahead, so inserts never hit a missing partition.
func (s *Store) EnsurePartitions(ctx context.Context, now time.Time) error {
	today := now.UTC().Truncate(24 * time.Hour)
	for _, t := range partitionedTables {
		for d := -1; d <= 2; d++ {
			from := today.AddDate(0, 0, d)
			to := from.AddDate(0, 0, 1)
			// Names and bounds come from dates formatted here, never from input.
			stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s%s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
				t.name, from.Format(partitionSuffix), t.name, from.Format(time.RFC3339), to.Format(time.RFC3339))
			if _, err := s.pool.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("store: create partition: %w", err)
			}
		}
	}
	return nil
}

// DropOldPartitions drops partitions that lie completely before the
// retention period and returns their names.
func (s *Store) DropOldPartitions(ctx context.Context, now time.Time) ([]string, error) {
	var dropped []string
	for _, t := range partitionedTables {
		rows, err := s.pool.Query(ctx, `
			SELECT c.relname FROM pg_inherits i
			JOIN pg_class c ON c.oid = i.inhrelid JOIN pg_class p ON p.oid = i.inhparent
			WHERE p.relname = $1`, t.name)
		if err != nil {
			return dropped, err
		}
		names, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return dropped, err
		}
		slices.Sort(names)
		cutoff := now.Add(-t.retention)
		for _, n := range names {
			day, err := time.Parse(partitionSuffix, strings.TrimPrefix(n, t.name))
			if err != nil {
				continue // not one of ours
			}
			if day.AddDate(0, 0, 1).After(cutoff) {
				continue
			}
			if _, err := s.pool.Exec(ctx, `DROP TABLE `+pgx.Identifier{n}.Sanitize()); err != nil {
				return dropped, fmt.Errorf("store: drop partition %s: %w", n, err)
			}
			dropped = append(dropped, n)
		}
	}
	return dropped, nil
}

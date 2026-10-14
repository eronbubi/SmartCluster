package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/eronbubi/smartcluster/control/internal/enroll"
	"github.com/eronbubi/smartcluster/pkg/ids"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

// Node is a registered node as stored.
type Node struct {
	ID           string
	Name         string
	Hostname     string
	Status       string
	OS           *proto.OSInfo
	Hardware     *proto.Hardware
	Capabilities []string
	AgentVersion string
	CertSerial   string
	CertNotAfter time.Time
	IPAddresses  []string
	MACAddresses []string
	CreatedAt    time.Time
	LastSeenAt   *time.Time
}

// Node status values.
const (
	StatusEnrolling = "enrolling"
	StatusOnline    = "online"
	StatusOffline   = "offline"
)

// CreateJoinCode stores the derived key of a new join code.
func (s *Store) CreateJoinCode(ctx context.Context, key []byte, expiresAt time.Time) (string, error) {
	id := ids.NewV7()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO enrollment_codes (id, purpose, code_key, expires_at) VALUES ($1, 'join', $2, $3)`, id, key, expiresAt)
	if err != nil {
		return "", fmt.Errorf("store: create join code: %w", err)
	}
	return id, nil
}

// ActiveCodes implements enroll.Store.
func (s *Store) ActiveCodes(ctx context.Context, now time.Time) ([]enroll.Code, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code_key, expires_at FROM enrollment_codes WHERE used_at IS NULL AND expires_at > $1`, now)
	if err != nil {
		return nil, fmt.Errorf("store: active codes: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (enroll.Code, error) {
		var c enroll.Code
		err := r.Scan(&c.ID, &c.Key, &c.ExpiresAt)
		return c, err
	})
}

// CompleteEnrollment implements enroll.Store: it consumes the code and
// creates the node in one transaction.
func (s *Store) CompleteEnrollment(ctx context.Context, codeID string, n enroll.NewNode) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE enrollment_codes SET used_at = now(), used_by_node = $2
			WHERE id = $1 AND used_at IS NULL AND expires_at > now()`, codeID, n.ID)
		if err != nil {
			return fmt.Errorf("store: consume code: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return enroll.ErrCodeUsed
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO nodes (id, name, status, cert_serial, cert_not_after) VALUES ($1, $2, 'enrolling', $3, $4)`,
			n.ID, n.Name, n.CertSerial, n.CertNotAfter)
		if err != nil {
			return fmt.Errorf("store: create node: %w", err)
		}
		return nil
	})
}

const nodeColumns = `id, name, coalesce(hostname, ''), status, os, hardware, capabilities, coalesce(agent_version, ''),
	cert_serial, cert_not_after, ip_addresses, mac_addresses, created_at, last_seen_at`

func scanNode(r pgx.Row) (Node, error) {
	var n Node
	var osJSON, hwJSON []byte
	var ips []netip.Prefix
	var macs []net.HardwareAddr
	err := r.Scan(&n.ID, &n.Name, &n.Hostname, &n.Status, &osJSON, &hwJSON, &n.Capabilities, &n.AgentVersion,
		&n.CertSerial, &n.CertNotAfter, &ips, &macs, &n.CreatedAt, &n.LastSeenAt)
	if err != nil {
		return n, err
	}
	if osJSON != nil {
		n.OS = &proto.OSInfo{}
		if err := json.Unmarshal(osJSON, n.OS); err != nil {
			return n, err
		}
	}
	if hwJSON != nil {
		n.Hardware = &proto.Hardware{}
		if err := json.Unmarshal(hwJSON, n.Hardware); err != nil {
			return n, err
		}
	}
	n.IPAddresses = make([]string, len(ips))
	for i, p := range ips {
		n.IPAddresses[i] = p.String()
	}
	n.MACAddresses = make([]string, len(macs))
	for i, m := range macs {
		n.MACAddresses[i] = m.String()
	}
	return n, nil
}

// ListNodes returns all nodes that were not removed, oldest first.
func (s *Store) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE removed_at IS NULL ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list nodes: %w", err)
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Node, error) { return scanNode(r) })
}

// GetNode returns one node that was not removed.
func (s *Store) GetNode(ctx context.Context, id string) (*Node, error) {
	if !ids.IsUUID(id) {
		return nil, ErrNotFound
	}
	n, err := scanNode(s.pool.QueryRow(ctx, `SELECT `+nodeColumns+` FROM nodes WHERE id = $1 AND removed_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get node: %w", err)
	}
	return &n, nil
}

// RecordHello stores what an agent reported on connect and marks it online.
func (s *Store) RecordHello(ctx context.Context, id string, h proto.Hello, now time.Time) error {
	osJSON, err := json.Marshal(h.OS)
	if err != nil {
		return err
	}
	hwJSON, err := json.Marshal(h.Hardware)
	if err != nil {
		return err
	}
	ips := []netip.Prefix{}
	macs := []net.HardwareAddr{}
	for _, nic := range h.Hardware.NICs {
		if mac, err := net.ParseMAC(nic.MAC); err == nil {
			macs = append(macs, mac)
		}
		for _, a := range nic.Addrs {
			if p, err := netip.ParsePrefix(a); err == nil {
				ips = append(ips, p)
			}
		}
	}
	caps := h.Capabilities
	if caps == nil {
		caps = []string{}
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE nodes SET hostname = $2, os = $3, hardware = $4, capabilities = $5, agent_version = $6,
			ip_addresses = $7, mac_addresses = $8, status = 'online', last_seen_at = $9
		WHERE id = $1 AND removed_at IS NULL`,
		id, h.OS.Hostname, osJSON, hwJSON, caps, h.AgentVersion, ips, macs, now)
	if err != nil {
		return fmt.Errorf("store: record hello: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetNodeOffline marks a node offline.
func (s *Store) SetNodeOffline(ctx context.Context, id string, lastSeen time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET status = 'offline', last_seen_at = $2 WHERE id = $1 AND status = 'online'`, id, lastSeen)
	return err
}

// MarkAllOffline is called at startup: no agent is connected yet.
func (s *Store) MarkAllOffline(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE nodes SET status = 'offline' WHERE status = 'online'`)
	return err
}

// UpdateNodeCert records a renewed agent certificate.
func (s *Store) UpdateNodeCert(ctx context.Context, id, serial string, notAfter time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE nodes SET cert_serial = $2, cert_not_after = $3 WHERE id = $1 AND removed_at IS NULL`, id, serial, notAfter)
	if err != nil {
		return fmt.Errorf("store: update node certificate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

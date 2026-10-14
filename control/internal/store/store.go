// Package store is the control center's persistence layer on Postgres.
package store

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eronbubi/smartcluster/control/internal/pki"
	"github.com/eronbubi/smartcluster/control/internal/secrets"
	"github.com/eronbubi/smartcluster/pkg/ids"
)

var (
	ErrNotFound  = errors.New("store: not found")
	ErrNoCluster = errors.New("store: cluster not initialized")
)

// Store wraps the connection pool and the secret box.
type Store struct {
	pool *pgxpool.Pool
	box  *secrets.Box
}

// New returns a Store.
func New(pool *pgxpool.Pool, box *secrets.Box) *Store {
	return &Store{pool: pool, box: box}
}

// Ping checks the database connection.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Cluster is the identity of the cluster this control center manages.
type Cluster struct {
	ID       string
	Name     string
	CA       *pki.CA
	CmdKeyID string
	CmdKey   ed25519.PrivateKey
}

const (
	secretScopeCluster = "cluster"
	secretCAKey        = "ca_key"
	secretCmdKey       = "cmd_key"
)

// CreateCluster stores a new cluster with its CA and command key.
func (s *Store) CreateCluster(ctx context.Context, c *Cluster) error {
	caKey, err := x509.MarshalPKCS8PrivateKey(c.CA.Key)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO cluster (id, name, ca_cert, cmd_key_id) VALUES ($1, $2, $3, $4)`,
			c.ID, c.Name, c.CA.Cert.Raw, c.CmdKeyID); err != nil {
			return fmt.Errorf("store: create cluster: %w", err)
		}
		if err := s.putSecret(ctx, tx, secretScopeCluster, secretCAKey, caKey); err != nil {
			return err
		}
		return s.putSecret(ctx, tx, secretScopeCluster, secretCmdKey, c.CmdKey.Seed())
	})
}

// LoadCluster reads the cluster and decrypts its keys.
func (s *Store) LoadCluster(ctx context.Context) (*Cluster, error) {
	var c Cluster
	var caDER []byte
	err := s.pool.QueryRow(ctx, `SELECT id, name, ca_cert, cmd_key_id FROM cluster`).Scan(&c.ID, &c.Name, &caDER, &c.CmdKeyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoCluster
	}
	if err != nil {
		return nil, fmt.Errorf("store: load cluster: %w", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, fmt.Errorf("store: CA certificate: %w", err)
	}
	keyDER, err := s.getSecret(ctx, secretScopeCluster, secretCAKey)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyDER)
	if err != nil {
		return nil, fmt.Errorf("store: CA key: %w", err)
	}
	caKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("store: CA key is not ECDSA")
	}
	seed, err := s.getSecret(ctx, secretScopeCluster, secretCmdKey)
	if err != nil {
		return nil, err
	}
	if len(seed) != ed25519.SeedSize {
		return nil, errors.New("store: command key has the wrong size")
	}
	c.CA = &pki.CA{Cert: caCert, Key: caKey}
	c.CmdKey = ed25519.NewKeyFromSeed(seed)
	return &c, nil
}

func secretAAD(scope, name string) []byte {
	return []byte(scope + "/" + name)
}

func (s *Store) putSecret(ctx context.Context, tx pgx.Tx, scope, name string, value []byte) error {
	nonce, ct := s.box.Seal(value, secretAAD(scope, name))
	_, err := tx.Exec(ctx, `
		INSERT INTO secrets (id, scope, name, ciphertext, nonce, key_version) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (scope, name) DO UPDATE SET ciphertext = EXCLUDED.ciphertext, nonce = EXCLUDED.nonce,
			key_version = EXCLUDED.key_version, rotated_at = now()`,
		ids.NewV7(), scope, name, ct, nonce, secrets.KeyVersion)
	if err != nil {
		return fmt.Errorf("store: put secret %s/%s: %w", scope, name, err)
	}
	return nil
}

func (s *Store) getSecret(ctx context.Context, scope, name string) ([]byte, error) {
	var ct, nonce []byte
	err := s.pool.QueryRow(ctx, `SELECT ciphertext, nonce FROM secrets WHERE scope = $1 AND name = $2`, scope, name).Scan(&ct, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("store: secret %s/%s: %w", scope, name, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: secret %s/%s: %w", scope, name, err)
	}
	return s.box.Open(nonce, ct, secretAAD(scope, name))
}

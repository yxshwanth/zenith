package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver for CockroachDB
)

// PoolConfig holds connection pool configuration
type PoolConfig struct {
	MaxOpenConns        int
	MaxIdleConns        int
	ConnMaxLifetime     time.Duration
	ConnMaxIdleTime     time.Duration
	HealthCheckInterval time.Duration
}

// DB wraps the database connection and provides Zookie utilities
type DB struct {
	conn            *sql.DB
	poolCfg         *PoolConfig
	stopHealthCheck chan struct{}
}

// NewDB creates a new database connection with default pool settings
func NewDB(connString string) (*DB, error) {
	return NewDBWithConfig(connString, &PoolConfig{
		MaxOpenConns:        25,
		MaxIdleConns:        5,
		ConnMaxLifetime:     5 * time.Minute,
		ConnMaxIdleTime:     1 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	})
}

// NewDBWithConfig creates a new database connection with custom pool settings
func NewDBWithConfig(connString string, poolCfg *PoolConfig) (*DB, error) {
	conn, err := sql.Open("pgx", connString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	conn.SetMaxOpenConns(poolCfg.MaxOpenConns)
	conn.SetMaxIdleConns(poolCfg.MaxIdleConns)
	conn.SetConnMaxLifetime(poolCfg.ConnMaxLifetime)
	if poolCfg.ConnMaxIdleTime > 0 {
		conn.SetConnMaxIdleTime(poolCfg.ConnMaxIdleTime)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{
		conn:            conn,
		poolCfg:         poolCfg,
		stopHealthCheck: make(chan struct{}),
	}

	// Start health check goroutine if interval is configured
	if poolCfg.HealthCheckInterval > 0 {
		go db.healthCheckLoop()
	}

	return db, nil
}

// healthCheckLoop periodically checks database connection health
func (db *DB) healthCheckLoop() {
	ticker := time.NewTicker(db.poolCfg.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := db.conn.PingContext(ctx); err != nil {
				// Log error but don't fail - connection pool will handle reconnection
				// In production, you might want to emit metrics here
			}
			cancel()
		case <-db.stopHealthCheck:
			return
		}
	}
}

// Close closes the database connection
func (db *DB) Close() error {
	close(db.stopHealthCheck)
	return db.conn.Close()
}

// Stats returns connection pool statistics
func (db *DB) Stats() sql.DBStats {
	return db.conn.Stats()
}

// GetZookie returns the current logical timestamp (Zookie) from CockroachDB
// CockroachDB returns cluster_logical_timestamp() as a DECIMAL, so we need to handle it as a string first
func (db *DB) GetZookie(ctx context.Context) (int64, error) {
	var zookieStr string
	err := db.conn.QueryRowContext(ctx, "SELECT cluster_logical_timestamp()::STRING").Scan(&zookieStr)
	if err != nil {
		return 0, fmt.Errorf("failed to get zookie: %w", err)
	}
	return ParseZookieString(zookieStr)
}

// ParseZookieString converts CockroachDB's DECIMAL timestamp string to int64.
// Format is typically "1766433684599320881.0000000000".
func ParseZookieString(zookieStr string) (int64, error) {
	var zookie int64
	if _, err := fmt.Sscanf(zookieStr, "%d", &zookie); err == nil {
		return zookie, nil
	}
	var zookieFloat float64
	if _, err := fmt.Sscanf(zookieStr, "%f", &zookieFloat); err != nil {
		return 0, fmt.Errorf("failed to parse zookie %q: %w", zookieStr, err)
	}
	return int64(zookieFloat), nil
}

// ValidateZookie checks if a zookie value is reasonable
// Returns true if valid, false if suspicious
func ValidateZookie(zookie int64) bool {
	// Zookie should be positive (logical timestamps are always positive)
	if zookie < 0 {
		return false
	}
	// Zookie should not be unreasonably large (future check)
	// CockroachDB logical timestamps are in nanoseconds since epoch
	// A reasonable upper bound would be current time + some buffer
	// For now, we just check it's positive
	return true
}

// QueryWithZookie executes a query with AS OF SYSTEM TIME if zookie is provided
// If zookie is 0, executes the query normally
// For required_zookie, we ensure the query reads at least as fresh as that timestamp
// Uses MAX(zookie, current_time) to prevent time-travel bugs
func (db *DB) QueryWithZookie(ctx context.Context, zookie int64, query string, args ...interface{}) (*sql.Rows, error) {
	if zookie > 0 {
		// Validate zookie before using it
		if !ValidateZookie(zookie) {
			// Invalid zookie, log warning but proceed with current time
			// This shouldn't happen in normal operation
		}

		// Get current database time to prevent time-travel bugs
		// Use MAX(zookie, current_time) to ensure we never read data older than "now"
		currentTime, err := db.GetZookie(ctx)
		if err != nil {
			// If we can't get current time, fall back to using zookie directly
			// This is a safety fallback, but should rarely happen
			query = fmt.Sprintf("%s AS OF SYSTEM TIME %d", query, zookie)
		} else {
			// Use MAX(zookie, current_time) to prevent time-travel
			effectiveZookie := zookie
			if currentTime > zookie {
				effectiveZookie = currentTime
			}
			query = fmt.Sprintf("%s AS OF SYSTEM TIME %d", query, effectiveZookie)
		}
	}
	return db.conn.QueryContext(ctx, query, args...)
}

// QueryRowWithZookie executes a query that returns a single row with AS OF SYSTEM TIME if zookie is provided
// Uses MAX(zookie, current_time) to prevent time-travel bugs
func (db *DB) QueryRowWithZookie(ctx context.Context, zookie int64, query string, args ...interface{}) *sql.Row {
	if zookie > 0 {
		// Validate zookie before using it
		if !ValidateZookie(zookie) {
			// Invalid zookie, log warning but proceed with current time
			// This shouldn't happen in normal operation
		}

		// Get current database time to prevent time-travel bugs
		// Use MAX(zookie, current_time) to ensure we never read data older than "now"
		currentTime, err := db.GetZookie(ctx)
		if err != nil {
			// If we can't get current time, fall back to using zookie directly
			// This is a safety fallback, but should rarely happen
			query = fmt.Sprintf("%s AS OF SYSTEM TIME %d", query, zookie)
		} else {
			// Use MAX(zookie, current_time) to prevent time-travel
			effectiveZookie := zookie
			if currentTime > zookie {
				effectiveZookie = currentTime
			}
			query = fmt.Sprintf("%s AS OF SYSTEM TIME %d", query, effectiveZookie)
		}
	}
	return db.conn.QueryRowContext(ctx, query, args...)
}

// ExecWithZookie executes a query (INSERT/UPDATE/DELETE) and returns the zookie
func (db *DB) ExecWithZookie(ctx context.Context, query string, args ...interface{}) (int64, error) {
	_, err := db.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to execute query: %w", err)
	}

	// Get the zookie after the write
	zookie, err := db.GetZookie(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get zookie after write: %w", err)
	}

	return zookie, nil
}

// RunMigrations executes SQL migration files
func (db *DB) RunMigrations(ctx context.Context, migrations []string) error {
	for i, migration := range migrations {
		if _, err := db.conn.ExecContext(ctx, migration); err != nil {
			return fmt.Errorf("failed to run migration %d: %w", i+1, err)
		}
	}
	return nil
}

// Package dolt implements a store.Backend that reads from a Dolt SQL server.
package dolt

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gitrgoliveira/muster/internal/store"
	_ "github.com/go-sql-driver/mysql" // mysql driver
)

// Backend reads issues from a Dolt SQL server via the MySQL wire protocol.
type Backend struct {
	db *sql.DB
}

// NewDolt constructs a Backend by connecting to the given DSN.
// Returns a wrapped store.ErrStoreUnavailable on connection failure.
func NewDolt(ctx context.Context, dsn string) (*Backend, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", store.ErrStoreUnavailable, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w: %v", store.ErrStoreUnavailable, err)
	}
	return &Backend{db: db}, nil
}

// List returns issues matching the filter.
func (b *Backend) List(ctx context.Context, f store.Filter) ([]store.Issue, error) {
	q, args := buildListQuery(f)
	rows, err := b.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("dolt query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	issues := make([]store.Issue, 0)
	for rows.Next() {
		var iss store.Issue
		if err := scanIntoIssue(rows, &iss); err != nil {
			return nil, err
		}
		if f.TruncateDesc > 0 && len(iss.Description) > f.TruncateDesc {
			iss.Description = iss.Description[:f.TruncateDesc]
		}
		issues = append(issues, iss)
		if f.Limit > 0 && len(issues) >= f.Limit {
			break
		}
	}
	return issues, rows.Err()
}

// Get returns the issue with the given ID, or store.ErrNotFound.
func (b *Backend) Get(ctx context.Context, id string) (*store.Issue, error) {
	row := b.db.QueryRowContext(ctx, getSQL, id)
	var iss store.Issue
	if err := scanIntoIssue(row, &iss); err != nil {
		if err == sql.ErrNoRows {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("dolt get: %w", err)
	}
	return &iss, nil
}

// Ping checks the database connection.
func (b *Backend) Ping(ctx context.Context) error {
	return b.db.PingContext(ctx)
}

// Close closes the database connection pool.
func (b *Backend) Close() error {
	return b.db.Close()
}

func buildListQuery(f store.Filter) (string, []any) {
	var where []string
	var args []any

	if len(f.Status) > 0 {
		placeholders := make([]string, len(f.Status))
		for i, s := range f.Status {
			placeholders[i] = "?"
			args = append(args, s)
		}
		where = append(where, "status IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(f.IDs) > 0 {
		placeholders := make([]string, len(f.IDs))
		for i, id := range f.IDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, "id IN ("+strings.Join(placeholders, ",")+")")
	}

	q := listSQL
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	return q, args
}

type scanner interface {
	Scan(dest ...any) error
}

func scanIntoIssue(s scanner, iss *store.Issue) error {
	return s.Scan(
		&iss.ID, &iss.Title, &iss.Description, &iss.Status,
		&iss.Priority, &iss.IssueType, &iss.Assignee, &iss.Owner,
		&iss.CreatedAt, &iss.UpdatedAt, &iss.StartedAt, &iss.ClosedAt,
		&iss.CloseReason, &iss.DependencyCount, &iss.DependentCount, &iss.CommentCount,
		&iss.Notes,
	)
}

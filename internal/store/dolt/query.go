package dolt

// Optional text columns are wrapped in COALESCE so a SQL NULL scans into an
// empty Go string instead of failing with "converting NULL to string is
// unsupported". Required columns (id, title, status, issue_type) are left bare.
const listSQL = `
SELECT id, title, COALESCE(description, ''), status, priority, issue_type,
       COALESCE(assignee, ''), COALESCE(owner, ''),
       created_at, updated_at, started_at, closed_at, COALESCE(close_reason, ''),
       dependency_count, dependent_count, comment_count, COALESCE(notes, '')
FROM issues`

const getSQL = `
SELECT id, title, COALESCE(description, ''), status, priority, issue_type,
       COALESCE(assignee, ''), COALESCE(owner, ''),
       created_at, updated_at, started_at, closed_at, COALESCE(close_reason, ''),
       dependency_count, dependent_count, comment_count, COALESCE(notes, '')
FROM issues WHERE id = ? LIMIT 1`

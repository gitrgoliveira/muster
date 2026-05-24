package dolt

const listSQL = `
SELECT id, title, description, status, priority, issue_type, assignee, owner,
       created_at, updated_at, started_at, closed_at, close_reason,
       dependency_count, dependent_count, comment_count, notes
FROM issues`

const getSQL = `
SELECT id, title, description, status, priority, issue_type, assignee, owner,
       created_at, updated_at, started_at, closed_at, close_reason,
       dependency_count, dependent_count, comment_count, notes
FROM issues WHERE id = ? LIMIT 1`

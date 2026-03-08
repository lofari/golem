package graph

import (
	"database/sql"

	"github.com/lofari/golem/internal/graph/model"
)

// InsertExecution inserts an execution record.
func (s *Store) InsertExecution(exec model.Execution) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO executions (session_id, started_at, ended_at, status) VALUES (?, ?, ?, ?)",
		exec.SessionID, exec.StartedAt, exec.EndedAt, exec.Status,
	)
	return err
}

// FinalizeExecution updates an execution's end time and status.
func (s *Store) FinalizeExecution(sessionID string, endedAt int64, status string) error {
	_, err := s.db.Exec(
		"UPDATE executions SET ended_at = ?, status = ? WHERE session_id = ?",
		endedAt, status, sessionID,
	)
	return err
}

// QueryExecutions returns the most recent executions, ordered by start time descending.
func (s *Store) QueryExecutions(limit int) ([]model.Execution, error) {
	rows, err := s.db.Query(
		"SELECT session_id, started_at, COALESCE(ended_at, 0), status FROM executions ORDER BY started_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execs []model.Execution
	for rows.Next() {
		var e model.Execution
		if err := rows.Scan(&e.SessionID, &e.StartedAt, &e.EndedAt, &e.Status); err != nil {
			return nil, err
		}
		execs = append(execs, e)
	}
	return execs, rows.Err()
}

// LatestExecution returns the most recent execution, or nil if none exist.
func (s *Store) LatestExecution() (*model.Execution, error) {
	execs, err := s.QueryExecutions(1)
	if err != nil {
		return nil, err
	}
	if len(execs) == 0 {
		return nil, nil
	}
	return &execs[0], nil
}

// ExecutionCount returns the number of stored executions.
func (s *Store) ExecutionCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM executions").Scan(&count)
	return count, err
}

// InsertCommand inserts a command record.
func (s *Store) InsertCommand(cmd model.Command) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO commands (id, session_id, seq, command, exit_code, working_dir) VALUES (?, ?, ?, ?, ?, ?)",
		cmd.ID, cmd.SessionID, cmd.Seq, cmd.Command, cmd.ExitCode, cmd.WorkingDir,
	)
	return err
}

// QueryCommandsBySession returns all commands for a session, ordered by sequence.
func (s *Store) QueryCommandsBySession(sessionID string) ([]model.Command, error) {
	rows, err := s.db.Query(
		"SELECT id, session_id, seq, command, COALESCE(exit_code, 0), COALESCE(working_dir, '') FROM commands WHERE session_id = ? ORDER BY seq",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cmds []model.Command
	for rows.Next() {
		var c model.Command
		if err := rows.Scan(&c.ID, &c.SessionID, &c.Seq, &c.Command, &c.ExitCode, &c.WorkingDir); err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}

// QueryFailedCommands returns commands with non-zero exit codes for a session.
func (s *Store) QueryFailedCommands(sessionID string) ([]model.Command, error) {
	rows, err := s.db.Query(
		"SELECT id, session_id, seq, command, exit_code, COALESCE(working_dir, '') FROM commands WHERE session_id = ? AND exit_code != 0 ORDER BY seq",
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cmds []model.Command
	for rows.Next() {
		var c model.Command
		if err := rows.Scan(&c.ID, &c.SessionID, &c.Seq, &c.Command, &c.ExitCode, &c.WorkingDir); err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}

// InsertOutput inserts a command output record.
func (s *Store) InsertOutput(out model.Output) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO outputs (command_id, stdout, stderr, truncated) VALUES (?, ?, ?, ?)",
		out.CommandID, out.Stdout, out.Stderr, out.Truncated,
	)
	return err
}

// QueryOutput returns the output for a given command ID.
func (s *Store) QueryOutput(commandID string) (*model.Output, error) {
	var o model.Output
	err := s.db.QueryRow(
		"SELECT command_id, COALESCE(stdout, ''), COALESCE(stderr, ''), truncated FROM outputs WHERE command_id = ?",
		commandID,
	).Scan(&o.CommandID, &o.Stdout, &o.Stderr, &o.Truncated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// InsertTestResult inserts a test result record.
func (s *Store) InsertTestResult(tr model.TestResult) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO test_results (id, session_id, name, passed, duration_ms, output) VALUES (?, ?, ?, ?, ?, ?)",
		tr.ID, tr.SessionID, tr.Name, tr.Passed, tr.DurationMs, tr.Output,
	)
	return err
}

// QueryTestResults returns test results for a session, optionally filtered by status.
func (s *Store) QueryTestResults(sessionID string, status string) ([]model.TestResult, error) {
	var rows *sql.Rows
	var err error
	switch status {
	case "passed":
		rows, err = s.db.Query(
			"SELECT id, session_id, name, passed, COALESCE(duration_ms, 0), COALESCE(output, '') FROM test_results WHERE session_id = ? AND passed = TRUE ORDER BY name",
			sessionID,
		)
	case "failed":
		rows, err = s.db.Query(
			"SELECT id, session_id, name, passed, COALESCE(duration_ms, 0), COALESCE(output, '') FROM test_results WHERE session_id = ? AND passed = FALSE ORDER BY name",
			sessionID,
		)
	default:
		rows, err = s.db.Query(
			"SELECT id, session_id, name, passed, COALESCE(duration_ms, 0), COALESCE(output, '') FROM test_results WHERE session_id = ? ORDER BY name",
			sessionID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.TestResult
	for rows.Next() {
		var tr model.TestResult
		if err := rows.Scan(&tr.ID, &tr.SessionID, &tr.Name, &tr.Passed, &tr.DurationMs, &tr.Output); err != nil {
			return nil, err
		}
		results = append(results, tr)
	}
	return results, rows.Err()
}

// InsertError inserts an error record.
func (s *Store) InsertError(er model.Error) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO errors (id, command_id, message, stack_trace) VALUES (?, ?, ?, ?)",
		er.ID, er.CommandID, er.Message, er.StackTrace,
	)
	return err
}

// QueryErrorsBySession returns all errors for a session.
func (s *Store) QueryErrorsBySession(sessionID string) ([]model.Error, error) {
	rows, err := s.db.Query(
		`SELECT e.id, e.command_id, e.message, COALESCE(e.stack_trace, '')
		 FROM errors e
		 JOIN commands c ON e.command_id = c.id
		 WHERE c.session_id = ?
		 ORDER BY c.seq`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var errors []model.Error
	for rows.Next() {
		var er model.Error
		if err := rows.Scan(&er.ID, &er.CommandID, &er.Message, &er.StackTrace); err != nil {
			return nil, err
		}
		errors = append(errors, er)
	}
	return errors, rows.Err()
}

// DeleteExecution removes an execution and all its related data (commands, outputs, errors, test results, edges).
func (s *Store) DeleteExecution(sessionID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete edges referencing execution nodes
	_, err = tx.Exec(`DELETE FROM edges WHERE from_node LIKE ? OR to_node LIKE ?`,
		"cmd:"+sessionID+":%", "cmd:"+sessionID+":%")
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM edges WHERE from_node LIKE ? OR to_node LIKE ?`,
		"err:"+sessionID+":%", "err:"+sessionID+":%")
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM edges WHERE from_node LIKE ? OR to_node LIKE ?`,
		"test:"+sessionID+":%", "test:"+sessionID+":%")
	if err != nil {
		return err
	}

	// Delete outputs (via command IDs)
	_, err = tx.Exec(`DELETE FROM outputs WHERE command_id IN (SELECT id FROM commands WHERE session_id = ?)`, sessionID)
	if err != nil {
		return err
	}

	// Delete errors (via command IDs)
	_, err = tx.Exec(`DELETE FROM errors WHERE command_id IN (SELECT id FROM commands WHERE session_id = ?)`, sessionID)
	if err != nil {
		return err
	}

	// Delete test results
	_, err = tx.Exec(`DELETE FROM test_results WHERE session_id = ?`, sessionID)
	if err != nil {
		return err
	}

	// Delete commands
	_, err = tx.Exec(`DELETE FROM commands WHERE session_id = ?`, sessionID)
	if err != nil {
		return err
	}

	// Delete execution
	_, err = tx.Exec(`DELETE FROM executions WHERE session_id = ?`, sessionID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// CommandCount returns the total number of stored commands.
func (s *Store) CommandCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM commands").Scan(&count)
	return count, err
}

// FailedCommandCount returns the number of commands with non-zero exit codes.
func (s *Store) FailedCommandCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM commands WHERE exit_code != 0").Scan(&count)
	return count, err
}

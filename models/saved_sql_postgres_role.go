package models

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
)

// SavedSQLSchema is the PostgreSQL schema the server's tables live in, and the
// only one saved SQL may read from.
const SavedSQLSchema = "public"

// dangerousPredefinedRoles let a member read or write server files, run
// programs, or read or write every table whatever its grants.
var dangerousPredefinedRoles = []string{
	"pg_read_server_files",
	"pg_write_server_files",
	"pg_execute_server_program",
	"pg_read_all_data",
	"pg_write_all_data",
}

// ReadOnlyRoleCheckError is what VerifyPostgresReadOnlyRole returns when the
// role behind the read-only handle can do more than saved SQL may. Failures
// names every check that failed, each with what was found.
type ReadOnlyRoleCheckError struct {
	Role     string
	Failures []string
}

func (e *ReadOnlyRoleCheckError) Error() string {
	role := e.Role
	if role == "" {
		role = "(unknown)"
	}
	return fmt.Sprintf("the DB_READONLY_DSN role %s is not restricted to the saved-SQL tables: %s",
		role, strings.Join(e.Failures, "; "))
}

// maxListed caps how many tables or roles one failure names.
const maxListed = 10

func listNames(names []string) string {
	if len(names) <= maxListed {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:maxListed], ", "), len(names)-maxListed)
}

// VerifyPostgresReadOnlyRole checks, through the read-only handle itself, that
// the role it connects as can read the tables IsSavedSQLAllowedTable allows and
// nothing else. mainRole and mainDatabase are current_user and
// current_database() on the server's main connection. It returns nil, a
// *ReadOnlyRoleCheckError naming every check that failed, or the error that
// stopped it from checking.
//
// The role fails when it:
//   - is the main role, a member of it, or a member of any other non-predefined
//     role (a member can SET ROLE to it and use its grants);
//   - is a superuser or bypasses row-level security;
//   - is a member of a predefined role that reads or writes server files, runs
//     programs, or reads or writes every table;
//   - connects to a different database from the main connection;
//   - has CREATE on any schema;
//   - has INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES or TRIGGER on any table,
//     view or sequence, or SELECT or USAGE on any sequence;
//   - can SELECT any column of a table, view or materialized view the allow-list
//     does not allow.
func VerifyPostgresReadOnlyRole(ctx context.Context, ro *sqlx.DB, mainRole, mainDatabase string) error {
	var self struct {
		Role      string `db:"role"`
		Database  string `db:"database"`
		Super     bool   `db:"rolsuper"`
		BypassRLS bool   `db:"rolbypassrls"`
	}
	if err := ro.GetContext(ctx, &self, `SELECT current_user AS role, current_database() AS database,
		r.rolsuper, r.rolbypassrls FROM pg_roles r WHERE r.rolname = current_user`); err != nil {
		return fmt.Errorf("connect to DB_READONLY_DSN to check its role: %w", err)
	}

	result := &ReadOnlyRoleCheckError{Role: self.Role}
	fail := func(format string, args ...any) {
		result.Failures = append(result.Failures, fmt.Sprintf(format, args...))
	}

	if self.Role == mainRole {
		fail("it is the main DB_DSN role")
	}
	if self.Super {
		fail("it is a superuser")
	}
	if self.BypassRLS {
		fail("it has BYPASSRLS")
	}
	if self.Database != mainDatabase {
		fail("it connects to database %q, not %q", self.Database, mainDatabase)
	}

	var memberOf []string
	if err := ro.SelectContext(ctx, &memberOf, `SELECT r.rolname FROM pg_roles r
		WHERE r.rolname <> current_user AND pg_has_role(current_user, r.oid, 'MEMBER')
		ORDER BY r.rolname`); err != nil {
		return fmt.Errorf("check the DB_READONLY_DSN role's memberships: %w", err)
	}
	var dangerous, other []string
	for _, role := range memberOf {
		switch {
		case role == mainRole:
			fail("it is a member of the main DB_DSN role %s", role)
		case isDangerousPredefinedRole(role):
			dangerous = append(dangerous, role)
		case strings.HasPrefix(role, "pg_"):
			// The other predefined roles (pg_monitor, pg_database_owner, ...)
			// grant no table access.
		default:
			other = append(other, role)
		}
	}
	if len(dangerous) > 0 {
		fail("it is a member of %s", listNames(dangerous))
	}
	if len(other) > 0 {
		fail("it is a member of role(s) %s, whose grants it can use", listNames(other))
	}

	var creatable []string
	if err := ro.SelectContext(ctx, &creatable, `SELECT n.nspname FROM pg_namespace n
		WHERE n.nspname <> 'information_schema' AND n.nspname NOT LIKE 'pg\_%'
		AND has_schema_privilege(n.oid, 'CREATE') ORDER BY n.nspname`); err != nil {
		return fmt.Errorf("check the DB_READONLY_DSN role's schema privileges: %w", err)
	}
	if len(creatable) > 0 {
		fail("it has CREATE on schema %s", listNames(creatable))
	}

	// Every relation outside the system schemas. Sequences answer only to
	// has_sequence_privilege; column-level grants only to has_any_column_privilege,
	// which has_table_privilege does not see.
	var relations []struct {
		Schema    string `db:"nspname"`
		Name      string `db:"relname"`
		Kind      string `db:"relkind"`
		CanRead   bool   `db:"can_read"`
		CanModify bool   `db:"can_modify"`
	}
	if err := ro.SelectContext(ctx, &relations, `SELECT n.nspname, c.relname, c.relkind::text AS relkind,
		CASE WHEN c.relkind = 'S' THEN has_sequence_privilege(c.oid, 'SELECT, USAGE')
		     ELSE has_any_column_privilege(c.oid, 'SELECT') END AS can_read,
		CASE WHEN c.relkind = 'S' THEN has_sequence_privilege(c.oid, 'UPDATE')
		     ELSE has_table_privilege(c.oid, 'INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER')
		          OR has_any_column_privilege(c.oid, 'INSERT, UPDATE, REFERENCES') END AS can_modify
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f', 'S')
		AND n.nspname <> 'information_schema' AND n.nspname NOT LIKE 'pg\_%'
		ORDER BY n.nspname, c.relname`); err != nil {
		return fmt.Errorf("check the DB_READONLY_DSN role's table privileges: %w", err)
	}
	var modifiable, readable []string
	for _, rel := range relations {
		name := rel.Schema + "." + rel.Name
		if rel.CanModify {
			modifiable = append(modifiable, name)
		}
		if !rel.CanRead {
			continue
		}
		allowed := rel.Kind != "S" && rel.Schema == SavedSQLSchema && IsSavedSQLAllowedTable(rel.Name) &&
			(rel.Kind == "r" || rel.Kind == "p")
		if !allowed {
			readable = append(readable, name)
		}
	}
	if len(modifiable) > 0 {
		fail("it can write to %s", listNames(modifiable))
	}
	if len(readable) > 0 {
		fail("it can SELECT from %s, which saved SQL may not read", listNames(readable))
	}

	if len(result.Failures) > 0 {
		return result
	}
	return nil
}

func isDangerousPredefinedRole(role string) bool {
	for _, d := range dangerousPredefinedRoles {
		if role == d {
			return true
		}
	}
	return false
}

// IsReadOnlyRoleCheckFailure reports whether err is a verdict on the role (as
// opposed to a failure to check it).
func IsReadOnlyRoleCheckFailure(err error) bool {
	var check *ReadOnlyRoleCheckError
	return errors.As(err, &check)
}

var postgresRoleName = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// PostgresReadOnlyRoleSQL returns the SQL that creates (or repairs) a role
// VerifyPostgresReadOnlyRole accepts: it may log in, may read the allowed
// tables, and may do nothing else. role must be a plain lower-case identifier.
func PostgresReadOnlyRoleSQL(role string) (string, error) {
	if !postgresRoleName.MatchString(role) {
		return "", fmt.Errorf("role name %q must be a lower-case identifier (letters, digits, underscores)", role)
	}
	quoted := `"` + role + `"`
	var b strings.Builder
	fmt.Fprintf(&b, `-- A read-only role for mahresources saved SQL queries (DB_READONLY_DSN).
--
-- Run this connected to the mahresources database, as the role that owns its
-- tables (the DB_DSN role) or as a superuser, after the server has started at
-- least once so that every table exists. Running it again is safe, and puts the
-- role back to exactly these grants.
--
-- The server checks this role at startup and again at most a minute before a
-- saved query runs. With -auth, saved SQL is refused until the check passes.

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%[1]s') THEN
    CREATE ROLE %[2]s LOGIN;
  END IF;
END
$$;
ALTER ROLE %[2]s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;

REVOKE ALL ON ALL TABLES IN SCHEMA %[3]s FROM %[2]s;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA %[3]s FROM %[2]s;
REVOKE CREATE ON SCHEMA %[3]s FROM %[2]s;
GRANT USAGE ON SCHEMA %[3]s TO %[2]s;

`, role, quoted, SavedSQLSchema)
	for _, table := range SavedSQLAllowedTables() {
		fmt.Fprintf(&b, "GRANT SELECT ON %s.%q TO %s;\n", SavedSQLSchema, table, quoted)
	}
	fmt.Fprintf(&b, `
-- Tables created later get no grant, so they stay unreadable; no ALTER DEFAULT
-- PRIVILEGES is needed.
--
-- Set the role's password, then point DB_READONLY_DSN at it:
--   \password %[1]s
--
-- The check also fails when the role can create objects in a schema. PostgreSQL
-- 14 and older grant CREATE on schema public to PUBLIC; if the server reports
-- that, grant CREATE to the DB_DSN role explicitly and revoke it from PUBLIC:
--   GRANT CREATE ON SCHEMA public TO <db_dsn_role>;
--   REVOKE CREATE ON SCHEMA public FROM PUBLIC;
-- It fails too when the role is a member of another role; revoke that
-- membership (REVOKE <other_role> FROM %[1]s).
`, role)
	return b.String(), nil
}

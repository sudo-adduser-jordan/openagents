// Package sqlite owns SQLite connection setup and goose-managed schema
// migrations. Typed CRUD lives in the store subpackage; this package keeps the
// public Open entrypoint and compatibility aliases for callers.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pressly/goose/v3"

	sqlitestore "github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite/store"

	// modernc.org/sqlite is the pure-Go (CGO-free) SQLite driver — chosen so the
	// daemon cross-compiles and ships as a static binary with no libsqlite/CGO
	// toolchain dependency, at the cost of some raw throughput vs a C-backed driver.
	_ "modernc.org/sqlite"
)

// Store is the SQLite-backed persistence layer.
type Store = sqlitestore.Store

//go:embed migrations/*.sql
var migrationsFS embed.FS

// pragmas are applied on every connection open. WAL + NORMAL lets readers run
// concurrently with the writer; busy_timeout absorbs brief writer contention;
// foreign_keys enforces the cascades and the CDC triggers' lookups.
const pragmas = "?_pragma=journal_mode(WAL)" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=foreign_keys(ON)" +
	"&_pragma=synchronous(NORMAL)"

const readOnlyPragmas = "?mode=ro" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=foreign_keys(ON)"

// maxReaders caps the reader pool. WAL allows many concurrent readers.
const maxReaders = 8

// databaseURI preserves filesystem characters instead of interpreting them as
// SQLite URI parameters/fragments. Both pools and read-only opens must address
// the same literal file, including percent signs and Windows drive paths.
func databaseURI(dataDir string) string {
	file := url.URL{Path: filepath.Join(dataDir, "open-agents.db")}
	return "file:" + file.EscapedPath()
}

// An older raw file: URI may have stored this directory's data elsewhere. Do
// not silently replace that database with an empty one after fixing the URI.
func checkLegacyDatabasePath(dataDir string) error {
	intended := filepath.Join(dataDir, "open-agents.db")
	if _, err := os.Stat(intended); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect intended database: %w", err)
	}
	raw := intended
	if end := strings.IndexAny(raw, "?#"); end >= 0 {
		raw = raw[:end]
	}
	var decoded strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '%' && i+2 < len(raw) {
			if value, err := url.PathUnescape(raw[i : i+3]); err == nil {
				if value[0] == 0 {
					break
				}
				decoded.WriteString(value)
				i += 2
				continue
			}
		}
		decoded.WriteByte(raw[i])
	}
	legacy := decoded.String()
	if legacy == intended {
		return nil
	}
	info, err := os.Stat(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect legacy database path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	file, err := os.Open(legacy)
	if err != nil {
		return fmt.Errorf("inspect legacy database: %w", err)
	}
	var header [16]byte
	n, readErr := io.ReadFull(file, header[:])
	_ = file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return fmt.Errorf("inspect legacy database: %w", readErr)
	}
	if n == len(header) && string(header[:]) == "SQLite format 3\x00" {
		return fmt.Errorf("legacy SQLite path parsing stored data at %q; refusing to create an empty database at %q: stop Open Agents and recover the legacy database explicitly before retrying", legacy, intended)
	}
	return nil
}

// Open opens (creating if absent) the SQLite database under dataDir and returns
// a Store. It uses TWO pools against the same file:
//
//   - a single WRITER connection (writeDB, MaxOpenConns=1): every write goes
//     here, so a write and the CDC triggers' subqueries it fires always see the
//     prior writes on the same connection (read-your-writes). This is required
//     because the pr/pr_checks triggers SELECT from sessions/pr to fill in the
//     event's project_id; a pooled writer could land that read on a connection
//     that hasn't caught up to the commit and read NULL.
//   - a READER pool (readDB, MaxOpenConns=maxReaders): all reads scale across
//     it; WAL readers see the latest committed snapshot.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := checkLegacyDatabasePath(dataDir); err != nil {
		return nil, err
	}
	dsn := databaseURI(dataDir) + pragmas

	writeDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite writer: %w", err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)
	if err := migrate(writeDB); err != nil {
		_ = writeDB.Close()
		return nil, err
	}

	readDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open sqlite reader: %w", err)
	}
	readDB.SetMaxOpenConns(maxReaders)
	readDB.SetMaxIdleConns(maxReaders)

	return sqlitestore.NewStore(writeDB, readDB), nil
}

// OpenReadOnly opens an existing SQLite database under dataDir without creating
// the directory, opening a writable connection, or running migrations.
func OpenReadOnly(ctx context.Context, dataDir string) (*Store, error) {
	dsn := databaseURI(dataDir) + readOnlyPragmas

	writeDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite read-only writer: %w", err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)
	if err := writeDB.PingContext(ctx); err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open sqlite read-only writer: %w", err)
	}

	readDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open sqlite read-only reader: %w", err)
	}
	readDB.SetMaxOpenConns(maxReaders)
	readDB.SetMaxIdleConns(maxReaders)
	if err := readDB.PingContext(ctx); err != nil {
		_ = readDB.Close()
		_ = writeDB.Close()
		return nil, fmt.Errorf("open sqlite read-only reader: %w", err)
	}

	return sqlitestore.NewStore(writeDB, readDB), nil
}

// gooseMu serialises calls into goose. goose v3 keeps its baseFS / logger /
// dialect as package-level globals (goose.SetBaseFS, goose.SetLogger,
// goose.SetDialect), so two concurrent Open() calls — uncommon in production
// but normal in -race test runs — race on those writes. The cost of holding the
// mutex is one process-startup migration; readers and writers afterwards never
// touch goose.
var gooseMu sync.Mutex

// cachedMigrationVersion holds the one-time computed expected migration version.
// The first call to expectedMigrationVersion populates it; subsequent calls
// return the cached value without re-scanning embedded files or touching goose
// globals.
var cachedMigrationVersion struct {
	sync.Once
	version int64
	err     error
}

// expectedMigrationVersion returns the highest version number among the
// embedded migration files. This is the version a fully-migrated database must
// have recorded as applied in goose_db_version.
//
// The result is computed once and cached for the lifetime of the process.
func expectedMigrationVersion() (int64, error) {
	cachedMigrationVersion.Do(func() {
		cachedMigrationVersion.err = computeExpectedMigrationVersion()
	})
	return cachedMigrationVersion.version, cachedMigrationVersion.err
}

func computeExpectedMigrationVersion() error {
	gooseMu.Lock()
	defer gooseMu.Unlock()
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	migrations, err := goose.CollectMigrations("migrations", 0, goose.MaxVersion)
	if err != nil {
		return fmt.Errorf("collect migrations: %w", err)
	}
	if len(migrations) == 0 {
		return fmt.Errorf("no embedded migrations found")
	}
	cachedMigrationVersion.version = migrations[len(migrations)-1].Version
	return nil
}

// OpenPreMigrated opens an already-fully-migrated SQLite database under
// dataDir, skipping all migration and repair logic. It is intended for test
// helpers that clone a known-good template database and need to open the copy
// without paying the ~55 ms migration overhead on every clone.
//
// It verifies that the database's goose_db_version records the expected
// current migration version; if the database is stale or has never been
// migrated, it returns an error so the caller can fall back to the production
// Open path rather than silently using an incompatible schema.
//
// Migration tests and any code that needs the production startup path must
// continue to call Open, not this function.
func OpenPreMigrated(dataDir string) (*Store, error) {
	want, err := expectedMigrationVersion()
	if err != nil {
		return nil, fmt.Errorf("determine expected migration version: %w", err)
	}

	dsn := databaseURI(dataDir) + pragmas

	writeDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite writer: %w", err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)

	var got int64
	if err := writeDB.QueryRow(
		`SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = 1`,
	).Scan(&got); err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("read applied migration version: %w", err)
	}
	if got != want {
		_ = writeDB.Close()
		return nil, fmt.Errorf(
			"database schema version mismatch: database has version %d but binary expects %d; "+
				"the template is stale — rebuild it with a full sqlite.Open call",
			got, want,
		)
	}

	readDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("open sqlite reader: %w", err)
	}
	readDB.SetMaxOpenConns(maxReaders)
	readDB.SetMaxIdleConns(maxReaders)

	return sqlitestore.NewStore(writeDB, readDB), nil
}

func migrate(db *sql.DB) error {
	gooseMu.Lock()
	defer gooseMu.Unlock()
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := rejectRetiredChainDatabase(db); err != nil {
		return err
	}
	// No WithAllowMissing: it existed so a build that had advanced past a
	// migration later added or renumbered upstream could still start. The
	// chain is append-only now, so an out-of-order ledger is a real fault and
	// goose rejecting it is the behaviour we want.
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// rejectRetiredChainDatabase refuses to start on a database whose goose ledger
// records applied versions this build no longer ships.
//
// The 139-file chain that preceded the baseline was development history:
// renumbered versions, 16 burned version numbers, and databases that only ever
// existed on developer machines and fast-moving Nightly builds. Those ledgers
// record applied version ids far beyond anything the baseline declares, and
// goose cannot reconcile them forward — it would treat them as already applied
// and leave the schema untouched.
//
// The test is set membership rather than a version threshold: every applied
// version must be declared by an embedded migration. A legitimate database
// migrated by any future build satisfies this automatically, so the check
// stays correct as versions 2, 3, ... are appended and never needs updating.
//
// Failing here is deliberate and loud. Silently accepting the database risks
// the daemon reporting healthy against a schema it does not understand; a
// developer or pilot user deleting one local database file costs them their
// local session history and nothing else.
func rejectRetiredChainDatabase(db *sql.DB) error {
	var ledgerTable int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`,
	).Scan(&ledgerTable); err != nil {
		return fmt.Errorf("inspect migration ledger: %w", err)
	}
	if ledgerTable == 0 {
		return nil // never migrated: a fresh database, goose builds it
	}

	migrations, err := goose.CollectMigrations("migrations", 0, goose.MaxVersion)
	if err != nil {
		return fmt.Errorf("collect migrations: %w", err)
	}
	// goose records its own version 0 bootstrap row in the ledger of every
	// database it has ever touched, and no migration file ever declares it.
	declared := map[int64]struct{}{0: {}}
	for _, m := range migrations {
		declared[m.Version] = struct{}{}
	}

	rows, err := db.Query(`SELECT DISTINCT version_id FROM goose_db_version WHERE is_applied = 1`)
	if err != nil {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var unknown []int64
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("read migration ledger: %w", err)
		}
		if _, ok := declared[version]; !ok {
			unknown = append(unknown, version)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	if len(unknown) == 0 {
		return nil
	}

	return fmt.Errorf(
		"this database was created by an older Open Agents build and its schema history "+
			"is not carried forward (unrecognised applied migration versions %v). Delete "+
			"open-agents.db in the Open Agents data directory to rebuild it; local session "+
			"history in it will be lost",
		unknown,
	)
}

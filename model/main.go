package model

import (
	"NewAPI-Gateway/common"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

// runMigrations handles custom migrations that AutoMigrate cannot handle
func runMigrations(db *gorm.DB) error {
	if err := repairDuplicateProviderTokens(db); err != nil {
		return fmt.Errorf("failed to repair duplicate provider tokens: %w", err)
	}
	if err := repairDuplicateModelRoutes(db); err != nil {
		return fmt.Errorf("failed to repair duplicate model routes: %w", err)
	}
	if err := createRouteUniquenessIndexes(db); err != nil {
		return fmt.Errorf("failed to create route uniqueness indexes: %w", err)
	}
	// Add client restriction columns to provider_token table if they don't exist
	if !db.Migrator().HasColumn(&ProviderToken{}, "allow_codex") {
		common.SysLog("Adding allow_codex column to provider_token table")
		if err := db.Migrator().AddColumn(&ProviderToken{}, "allow_codex"); err != nil {
			return fmt.Errorf("failed to add allow_codex column: %w", err)
		}
	}

	if !db.Migrator().HasColumn(&ProviderToken{}, "allow_cc") {
		common.SysLog("Adding allow_cc column to provider_token table")
		if err := db.Migrator().AddColumn(&ProviderToken{}, "allow_cc"); err != nil {
			return fmt.Errorf("failed to add allow_cc column: %w", err)
		}
	}

	if !db.Migrator().HasColumn(&ProviderToken{}, "block_clients") {
		common.SysLog("Adding block_clients column to provider_token table")
		if err := db.Migrator().AddColumn(&ProviderToken{}, "block_clients"); err != nil {
			return fmt.Errorf("failed to add block_clients column: %w", err)
		}
	}

	// Add security audit columns to llm_trace table if they don't exist
	if !db.Migrator().HasColumn(&LLMTrace{}, "risk_level") {
		common.SysLog("Adding risk_level column to llm_trace table")
		if err := db.Migrator().AddColumn(&LLMTrace{}, "risk_level"); err != nil {
			return fmt.Errorf("failed to add risk_level column: %w", err)
		}
	}

	if !db.Migrator().HasColumn(&LLMTrace{}, "risk_tags") {
		common.SysLog("Adding risk_tags column to llm_trace table")
		if err := db.Migrator().AddColumn(&LLMTrace{}, "risk_tags"); err != nil {
			return fmt.Errorf("failed to add risk_tags column: %w", err)
		}
	}

	if !db.Migrator().HasColumn(&LLMTrace{}, "auto_reviewed") {
		common.SysLog("Adding auto_reviewed column to llm_trace table")
		if err := db.Migrator().AddColumn(&LLMTrace{}, "auto_reviewed"); err != nil {
			return fmt.Errorf("failed to add auto_reviewed column: %w", err)
		}
	}

	// Clean up orphan routes: model_routes referencing deleted providers
	cleanupOrphanRoutes()

	return nil
}

func repairDuplicateProviderTokens(db *gorm.DB) error {
	type tokenKey struct {
		ID         int
		ProviderID int
		UpstreamID int
	}
	var tokens []tokenKey
	if err := db.Model(&ProviderToken{}).Select("id, provider_id, upstream_token_id").Order("id ASC").Find(&tokens).Error; err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(tokens))
	duplicateIDs := make([]int, 0)
	for _, token := range tokens {
		key := fmt.Sprintf("%d:%d", token.ProviderID, token.UpstreamID)
		if _, exists := seen[key]; exists {
			duplicateIDs = append(duplicateIDs, token.ID)
			continue
		}
		seen[key] = struct{}{}
	}
	for _, batch := range chunkInts(duplicateIDs, sqliteSingleInChunkSize) {
		if err := db.Where("provider_token_id IN ?", batch).Delete(&ModelRoute{}).Error; err != nil {
			return err
		}
		if err := db.Where("id IN ?", batch).Delete(&ProviderToken{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func repairDuplicateModelRoutes(db *gorm.DB) error {
	type routeKey struct {
		ID         int
		ProviderID int
		TokenID    int
		ModelName  string
	}
	var routes []routeKey
	if err := db.Model(&ModelRoute{}).Select("id, provider_id, provider_token_id, model_name").Order("id ASC").Find(&routes).Error; err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(routes))
	duplicateIDs := make([]int, 0)
	for _, route := range routes {
		key := fmt.Sprintf("%d:%d:%s", route.ProviderID, route.TokenID, route.ModelName)
		if _, exists := seen[key]; exists {
			duplicateIDs = append(duplicateIDs, route.ID)
			continue
		}
		seen[key] = struct{}{}
	}
	for _, batch := range chunkInts(duplicateIDs, sqliteSingleInChunkSize) {
		if err := db.Where("id IN ?", batch).Delete(&ModelRoute{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func createRouteUniquenessIndexes(db *gorm.DB) error {
	statements := []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_model_routes_provider_token_model ON model_routes(provider_id, provider_token_id, model_name)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_tokens_provider_upstream ON provider_tokens(provider_id, upstream_token_id)",
	}
	if db.Dialector.Name() == "mysql" {
		statements = nil
		if !db.Migrator().HasIndex(&ModelRoute{}, "idx_model_routes_provider_token_model") {
			statements = append(statements, "ALTER TABLE model_routes ADD UNIQUE INDEX idx_model_routes_provider_token_model (provider_id, provider_token_id, model_name)")
		}
		if !db.Migrator().HasIndex(&ProviderToken{}, "idx_provider_tokens_provider_upstream") {
			statements = append(statements, "ALTER TABLE provider_tokens ADD UNIQUE INDEX idx_provider_tokens_provider_upstream (provider_id, upstream_token_id)")
		}
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") && !strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			return err
		}
	}
	return nil
}

// cleanupOrphanRoutes removes model_routes that reference deleted providers
func cleanupOrphanRoutes() {
	deleted, err := CleanupOrphanModelRoutes()
	if err != nil {
		common.SysError(fmt.Sprintf("Failed to cleanup orphan routes: %v", err))
		return
	}
	if deleted > 0 {
		common.SysLog(fmt.Sprintf("Successfully cleaned up %d orphan routes", deleted))
	}
}

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
		}
		DB.Create(&rootUser)
	}
	return nil
}

func CountTable(tableName string) (num int64) {
	DB.Table(tableName).Count(&num)
	return
}

func normalizeSQLDriver(driver string) string {
	switch strings.TrimSpace(strings.ToLower(driver)) {
	case "mysql":
		return "mysql"
	case "postgres", "postgresql":
		return "postgres"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return ""
	}
}

func detectSQLDriver(dsn string) string {
	dsnLower := strings.ToLower(strings.TrimSpace(dsn))
	if strings.HasPrefix(dsnLower, "postgres://") || strings.HasPrefix(dsnLower, "postgresql://") {
		return "postgres"
	}
	if strings.Contains(dsnLower, "dbname=") && strings.Contains(dsnLower, "user=") {
		return "postgres"
	}
	return "mysql"
}

// GetDBInfo returns the current database driver and the sqlite path when applicable.
func GetDBInfo() (driver string, sqlitePath string) {
	dsn := strings.TrimSpace(os.Getenv("SQL_DSN"))
	configuredDriver := normalizeSQLDriver(os.Getenv("SQL_DRIVER"))
	driver = configuredDriver
	if driver == "" {
		if dsn == "" {
			driver = "sqlite"
		} else {
			driver = detectSQLDriver(dsn)
		}
	}
	if driver == "sqlite" {
		sqlitePath = common.SQLitePath
		if dsn != "" {
			sqlitePath = dsn
		}
	}
	return driver, sqlitePath
}

const (
	// sqliteBusyTimeoutMs makes a writer wait for the write lock instead of
	// failing immediately with SQLITE_BUSY. Without it, concurrent requests
	// surface "database is locked" errors as soon as two writes overlap.
	sqliteBusyTimeoutMs = 5000

	// sqliteMaxOpenConnsDefault keeps the pool small on purpose: SQLite
	// serialises writers, so a large pool only increases lock contention and
	// memory pressure. Readers still run concurrently under WAL.
	sqliteMaxOpenConnsDefault = 8
)

// buildSQLiteDSN appends the PRAGMAs that must be applied on every connection.
//
// journal_mode, synchronous and busy_timeout are connection-scoped, so they
// cannot be applied once with a single PRAGMA statement on a pooled handle —
// the go-sqlite3 driver applies DSN parameters per connection instead.
//
// WAL is the important one: the default rollback journal needs four fsyncs per
// commit, which made every usage-log write cost tens of milliseconds and
// serialised the whole gateway on a single write lock.
func buildSQLiteDSN(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = common.SQLitePath
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + fmt.Sprintf(
		"_busy_timeout=%d&_synchronous=NORMAL&_journal_mode=WAL",
		sqliteBusyTimeoutMs,
	)
}

// configureSQLiteConnection sizes the pool and logs the journal mode that
// actually took effect (WAL silently degrades on filesystems that do not
// support it, e.g. some network shares).
func configureSQLiteConnection(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	maxOpen := sqliteMaxOpenConnsDefault
	if raw := strings.TrimSpace(os.Getenv("SQL_MAX_OPEN_CONNS")); raw != "" {
		if parsed, convErr := strconv.Atoi(raw); convErr == nil && parsed > 0 {
			maxOpen = parsed
		} else {
			common.SysLog("invalid SQL_MAX_OPEN_CONNS, falling back to default: " + raw)
		}
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxOpen)

	var journalMode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&journalMode).Error; err != nil {
		common.SysLog("failed to read sqlite journal_mode: " + err.Error())
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(journalMode), "wal") {
		common.SysLog(fmt.Sprintf("sqlite journal_mode=%s (expected wal: WAL is unavailable on this filesystem)", journalMode))
		return nil
	}
	common.SysLog(fmt.Sprintf("sqlite tuned: journal_mode=wal busy_timeout=%dms max_open_conns=%d", sqliteBusyTimeoutMs, maxOpen))
	return nil
}

func InitDB() (err error) {
	var db *gorm.DB

	dsn := strings.TrimSpace(os.Getenv("SQL_DSN"))
	configuredDriver := normalizeSQLDriver(os.Getenv("SQL_DRIVER"))
	if os.Getenv("SQL_DRIVER") != "" && configuredDriver == "" {
		return fmt.Errorf("unsupported SQL_DRIVER: %s (supported: mysql, postgres, sqlite)", os.Getenv("SQL_DRIVER"))
	}

	driver := configuredDriver
	if driver == "" {
		if dsn == "" {
			driver = "sqlite"
		} else {
			driver = detectSQLDriver(dsn)
		}
	}

	switch driver {
	case "mysql":
		if dsn == "" {
			return fmt.Errorf("SQL_DSN is required when SQL_DRIVER=mysql")
		}
		common.SysLog("using MySQL as database")
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			PrepareStmt: true, // precompile SQL
		})
	case "postgres":
		if dsn == "" {
			return fmt.Errorf("SQL_DSN is required when SQL_DRIVER=postgres")
		}
		common.SysLog("using PostgreSQL as database")
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			PrepareStmt: true, // precompile SQL
		})
	case "sqlite":
		sqlitePath := common.SQLitePath
		if dsn != "" {
			sqlitePath = dsn
		}
		db, err = gorm.Open(sqlite.Open(buildSQLiteDSN(sqlitePath)), &gorm.Config{
			PrepareStmt: true, // precompile SQL
		})
		if err == nil {
			if poolErr := configureSQLiteConnection(db); poolErr != nil {
				common.SysLog("failed to configure sqlite connection pool: " + poolErr.Error())
			}
		}
		if configuredDriver == "" && dsn == "" {
			common.SysLog("SQL_DSN not set, using SQLite as database")
		} else {
			common.SysLog("using SQLite as database")
		}
	default:
		return fmt.Errorf("unsupported SQL driver: %s", driver)
	}

	if err == nil {
		DB = db
		err = db.AutoMigrate(&User{})
		if err != nil {
			return err
		}
		err = db.AutoMigrate(&Option{})
		if err != nil {
			return err
		}
		err = db.AutoMigrate(&Provider{})
		if err != nil {
			return err
		}
		err = db.AutoMigrate(&ProviderToken{})
		if err != nil {
			return err
		}
		err = db.AutoMigrate(&AggregatedToken{})
		if err != nil {
			return err
		}
		err = db.AutoMigrate(&ModelRoute{})
		if err != nil {
			return err
		}
		err = db.AutoMigrate(&SystemPrompt{})
		if err != nil {
			return err
		}
		err = db.AutoMigrate(&ModelPricing{})
		if err != nil {
			return err
		}
		err = db.AutoMigrate(&UsageLog{})
		if err != nil {
			return err
		}
		err = db.AutoMigrate(&LLMTrace{})
		if err != nil {
			return err
		}

		// Run migrations for new features
		err = runMigrations(db)
		if err != nil {
			return err
		}

		err = createRootAccountIfNeed()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func CloseDB() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

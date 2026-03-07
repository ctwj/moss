# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Moss is a lightweight content management system (CMS) built with Go backend and Vue 3 frontend. It features a plugin architecture, multi-database support, and internationalization.

## Architecture

### Backend Structure (Go)
- **Entry Point**: `main/cmd/web/main.go`
- **Layered Architecture**:
  - `api/web/` - Web API layer (controllers, DTOs, middleware, routers)
  - `application/` - Application services
  - `domain/` - Domain models and business logic
  - `infrastructure/` - Infrastructure layer (database, caching, etc.)
  - `plugins/` - Plugin system for extensible functionality
  - `themes/` - Theme templates
  - `resources/` - Static resources

### Frontend Structure (Vue 3)
- **Admin Panel**: `admin/` - Vue 3 + Vite + Tailwind CSS
- **Theme System**: `theme/` - Frontend themes (germ is default)

## Development Commands

### Essential Commands
```bash
# Install dependencies
task init-admin          # Frontend dependencies
cd main && go mod tidy   # Backend dependencies

# Development
task dev                 # Start full development environment (both frontend and backend with hot reload)
task run                 # Start backend only (no hot reload)
task admin               # Start frontend only

# Testing
cd main && go test ./... # Run all backend tests
cd main && go test ./plugins/... # Run plugin tests

# Build
task build               # Build both frontend and backend for production
task build-admin         # Build frontend only
task build-main          # Cross-compile backend for multiple platforms

# Utilities
task status              # Check development environment status
task logs                # View recent logs
task reset-admin         # Reset admin credentials (admin/admin123)
```

### Development Environment
- **Backend Hot Reload**: Uses Air tool (config: `main/.air.toml`)
- **Frontend Hot Reload**: Vite dev server with HMR
- **Default Ports**:
  - Backend: 9008
  - Frontend: 3000
  - Website: 80 (via Nginx)

## Plugin System

Plugins are located in `main/plugins/` and implement specific interfaces. Key plugin types:
- **Content Processing**: ArticleSanitizer, GenerateSlug, GenerateDescription
- **Media Processing**: SaveArticleImages, MakeCarousel
- **SEO**: PushToBaidu, PushToBing
- **Automation**: GnDownSpider, NewDidiAuto

Plugins are registered in `main/startup/startup.go`.

## Database Support

Supports SQLite (default), MySQL, and PostgreSQL. Configuration via `main/conf.toml`:
- SQLite: `./moss.db?_pragma=journal_mode(WAL)`
- MySQL: `user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True`
- PostgreSQL: `host=127.0.0.1 port=5432 user=postgres password=123456 dbname=moss sslmode=disable`

## Testing Patterns

Backend tests use Go's standard testing framework:
- Test files: `*_test.go`
- Run specific test: `go test -run TestFunctionName ./path/to/package`
- Plugin tests: Located alongside plugins (e.g., `gndown_plugin_test.go`)

## Frontend Development

The admin panel uses Vue 3 with Vite:
- **Hot Reload**: Automatically applies changes without restart
- **API Proxy**: `/admin/api/*` requests are proxied to backend
- **Build Output**: Static files served by backend

## Key Development Notes

1. **热重载**: Frontend changes are automatically applied - no restart needed
2. **Backend Hot Reload**: Air monitors `.go`, `.tpl`, `.tmpl`, `.html`, `.toml` files
3. **Plugin Development**: Create new plugins in `main/plugins/` and register in `startup.go`
4. **Database Migrations**: Handled automatically by GORM
5. **Configuration**: Use `main/conf.toml` for runtime configuration
6. **Multi-language**: Admin panel supports 12 languages
7. **Theme Development**: Themes in `theme/` directory with npm build process

## Common Development Tasks

### Adding a New Plugin
1. Create plugin file in `main/plugins/`
2. Implement required interface
3. Register in `main/startup/startup.go`
4. Add tests in `*_test.go` file

### Modifying Frontend
1. Edit files in `admin/src/`
2. Changes auto-reload via Vite
3. Build with `task build-admin`

### Database Changes
1. Update domain models in `main/domain/`
2. GORM handles migrations automatically
3. Test with different database types (SQLite/MySQL/PostgreSQL)

### API Development
1. Add controllers in `main/api/web/controller/`
2. Define DTOs in `main/api/web/dto/`
3. Update routers in `main/api/web/router/`
4. Add middleware if needed in `main/api/web/middleware/`
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Blueprint engine: YAML-defined pipelines that orchestrate Claude Code sessions with state management, control flow, and error recovery
- Custom predicates: expression-based predicates in blueprint YAML (`path.to.key == "value"`)
- Error handler priority chain: step-level > blueprint-level > built-in defaults, with `_error_context` injection on retries
- Knowledge graph: tree-sitter indexing, embeddings, LSP integration, 5-stage context ranking
- MCP server: structured state updates and graph query tools for Claude Code sessions
- Flutter desktop UI: run management, event timeline, terminal emulation
- WebSocket event broadcast: real-time engine event streaming for UI integration
- HTTP API server: REST endpoints for project state, config, processes, and graph queries

### Removed
- Clojure DSL runtime: patterns extracted into Go blueprint engine (custom predicates, error classification)

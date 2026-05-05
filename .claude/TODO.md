# Caido MCP Server TODO

- [x] [2026-04-28] Caido 0.56 compat verified -- SDK v0.5.0 union types, all 4 tested tools pass (list_requests, list_filters, list_tamper_rules, create_tamper_rule)
- [x] [2026-05-05] Chunk 1 test infra + CI complete (61 tests, schema contracts, GitHub Actions)
- [x] [2026-05-05] v2 roadmap spec patched (SDK mismatches, annotation corrections)
- [x] [2026-05-05] Chunk 2: create_replay_session tool (create + RenameReplaySession flow)
- [x] [2026-05-05] Chunk 3: MCP Resources (proxy history, sitemap, findings, replay sessions)
- [x] [2026-05-05] Chunk 4: Smarter responses (fingerprints, adaptive body limits, diff mode)
- [x] [2026-05-05] Add Windows binaries to release pipeline
- [x] [2026-05-05] Add MCP tools for unused SDK services: HostedFiles, Tasks, Plugins
- [ ] [2026-04-09] Add race condition support (research Caido API for low-level socket access)
- [ ] [2026-04-09] Fix oneof workarounds in delete_findings.go and export_findings.go when genqlient adds omitempty for nullable list fields

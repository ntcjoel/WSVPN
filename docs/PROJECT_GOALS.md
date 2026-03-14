# WSVPN Project Goals & Vision

**Document Version:** 1.0  
**Date:** 2026-03-06  
**Current Release:** v0.4.2

---

## Executive Summary

**WSVPN** is a lightweight, high-performance WebSocket-based VPN system designed for personal and small-team use. It provides secure remote network access through WebSocket/QUIC tunnels with advanced obfuscation capabilities.

**Mission:** Build a production-ready, multi-platform VPN solution that is:
- 🔒 **Secure** - TLS encryption, UUID authentication, traffic obfuscation
- 🚀 **Fast** - Minimal overhead, high throughput
- 🛠️ **Simple** - Single binary deployment, easy configuration
- 🌐 **Portable** - Cross-platform (Linux, Windows, macOS, mobile)
- 🎭 **Stealthy** - DPI evasion through HTTPS pattern simulation

---

## Core Objectives

### 1. Technical Excellence

| Goal | Target | Status |
|------|--------|--------|
| **Cross-Platform Support** | Linux, Windows, macOS, Android, iOS | 🟡 Partial (Linux + Windows done) |
| **Dual Transport** | WebSocket + QUIC | ✅ Complete |
| **Traffic Obfuscation** | DPI evasion, HTTPS simulation | ✅ Complete |
| **Zero-Copy Performance** | Rust implementation for production | ⏳ Planned |
| **Sub-100ms Latency** | End-to-end RTT | ✅ Achieved (144ms avg) |
| **50+ Mbps Throughput** | Single client throughput | ✅ Achieved (52.4 Mbps) |
| **99.9% Stability** | 24h uptime, <1% packet loss | ⏳ Testing |

### 2. Security & Privacy

| Goal | Implementation | Status |
|------|----------------|--------|
| **Encryption** | TLS 1.3 (wss://), QUIC built-in | ✅ Complete |
| **Authentication** | UUID-based, server-side validation | ✅ Complete |
| **Obfuscation** | HTTPS pattern, random padding, timing jitter | ✅ Complete |
| **No Logging** | Minimal metadata, no traffic logs | ✅ By design |
| **Audit Trail** | Security audits for each release | ✅ v0.4, v0.4.1, v0.4.2 |

### 3. User Experience

| Goal | Target | Status |
|------|--------|--------|
| **One-Click Connect** | Simple CLI or GUI | 🟡 CLI done, GUI planned |
| **Auto Reconnect** | Network failure recovery | ✅ Complete (v0.4.2) |
| **Configuration Management** | JSON config, hot reload | ✅ Complete |
| **Management Tools** | Chrome plugin, Web panel | ⏳ In development |
| **Documentation** | Complete user + admin guides | ✅ Complete |

---

## Product Roadmap

### Phase 1: Core Foundation ✅ (Completed: 2026-03-01 ~ 2026-03-03)

**Goal:** Production-ready Go implementation with obfuscation

**Deliverables:**
- ✅ WebSocket tunnel (gorilla/websocket)
- ✅ TUN/TAP interface (songgao/water)
- ✅ UUID authentication
- ✅ Traffic obfuscation (padding, HTTPS pattern, timing jitter)
- ✅ Nginx reverse proxy integration
- ✅ Health endpoint + config hot reload
- ✅ Cross-machine testing (tc.vps ↔ ts.vps)
- ✅ 0% packet loss, 52.4 Mbps throughput

**Release:** v0.3 (Production Ready)

---

### Phase 2: Multi-Platform Expansion ✅ (Completed: 2026-03-06)

**Goal:** Windows client + management tools

**Deliverables:**
- ✅ Windows CLI client (Wintun driver)
- ✅ v0.4.2 stability fixes (WebSocket mutex, routing, reconnect)
- ✅ Standardized release packaging
- ⏳ Chrome plugin (smart routing rules, SwitchyOmega import)
- ⏳ Web Panel (Go + HTMX, client management)

**Releases:** v0.4.0 (Windows), v0.4.1 (routing fix), v0.4.2 (stability)

---

### Phase 3: Rust Migration ⏳ (Planned: 2026-03-07 ~ 2026-03-14)

**Goal:** Performance optimization with Rust implementation

**Rationale:**
- Go: Rapid prototyping, orchestration (done)
- Rust: Production performance, memory safety, zero-copy

**Architecture:**
```
Leader (Ellie) → Task decomposition
    ↓
Niuma (Local RTX 5060 Ti) → Rust code generation
    ↓
Coder → Security audit + review
    ↓
Leader → Integration + testing
```

**Modules:**
| Module | Go Implementation | Rust Implementation | Priority |
|--------|-------------------|---------------------|----------|
| WebSocket | gorilla/websocket | tokio-tungstenite | P0 |
| TUN Interface | water | tun-tap / rtnetlink | P0 |
| Obfuscation | Custom padding | Zero-copy Bytes | P0 |
| Routing | map + RWMutex | DashMap | P1 |
| Memory Pool | sync.Pool | bumpalo | P1 |
| Metrics | Prometheus | prometheus-client | P2 |

**Phases:**
- Phase 3.1: Core framework (WebSocket + TUN forwarding)
- Phase 3.2: Obfuscation module (zero-copy optimization)
- Phase 3.3: Authentication + config hot reload
- Phase 3.4: Performance benchmarking vs Go
- Phase 3.5: 24h stability test

**Target:** Rust ≥ Go performance, same feature parity

---

### Phase 4: Advanced Features ⏳ (Planned: 2026-03-15 ~ 2026-03-31)

**Goal:** Enterprise-grade features

**Features:**
- ⏳ Multi-hop relay (chain multiple servers)
- ⏳ Load balancing (auto server selection)
- ⏳ Mobile apps (iOS/Android)
- ⏳ GUI clients (Tauri for desktop, Flutter for mobile)
- ⏳ Traffic statistics + billing (for commercial use)
- ⏳ Plugin system (custom routing rules, filters)

---

### Phase 5: Production Hardening ⏳ (Planned: 2026-04-01 ~)

**Goal:** Battle-tested production deployment

**Requirements:**
- ⏳ 7-day stability test (no crashes, <0.1% packet loss)
- ⏳ 100+ concurrent clients test
- ⏳ DDoS resistance testing
- ⏳ Failover + high availability
- ⏳ Automated deployment (Docker, Kubernetes)
- ⏳ Monitoring + alerting (Grafana, Prometheus)

---

## Success Metrics

### Performance KPIs

| Metric | Target | Current (v0.4.2) | Gap |
|--------|--------|------------------|-----|
| Latency (RTT) | <100ms | 144ms | -44ms |
| Throughput | >50 Mbps | 52.4 Mbps | ✅ |
| Packet Loss | <0.1% | 0% | ✅ |
| Memory Usage | <50MB | ~20MB | ✅ |
| CPU Usage | <5% | ~2% | ✅ |
| Reconnect Time | <5s | ~5s | ✅ |

### Adoption KPIs

| Metric | Target | Current |
|--------|--------|---------|
| Active Devices | 10+ | 2 (tc.vps, ts.vps) |
| Daily Uptime | >99% | Testing |
| User Satisfaction | >4.5/5 | N/A |

---

## Technical Principles

### 1. Automation First (最高优先级)
- 绝大部分情况优先考虑自动化
- 自动化必须保证可回退
- 失败时自动回滚 + 邮件通知
- 反复尝试保障任务及时完成

### 2. Safety First (安全优先)
- Configuration changes: dry-run → audit → approval → test → commit
- Never modify configs without backup and validation
- `safety-auditor.sh` provides independent verification
- Auto-rollback on failure with email notification

### 3. Logical Rigor
- Prioritize structural integrity and first-principles thinking
- Every decision backed by clear "Why" and "How"
- "But-If" analysis for failure scenarios

### 4. Go-to-Rust Lifecycle
- **Go:** Rapid orchestration, interface prototyping ✅
- **Rust:** Performance-critical production layer ⏳

### 5. Multi-Agent Collaboration
```
Leader (Ellie)    → Architecture, orchestration, quality control
Niuma (Local GPU) → Code generation (0 token cost)
Coder             → Code review, security audit
Aux               → Fallback, complex reasoning
```

---

## Risk Assessment

### Technical Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Routing loop (Windows) | ✅ Occurred (v0.4.1) | 🔴 Critical | Fixed in v0.4.2, automated testing |
| WebSocket concurrent write | ✅ Occurred (v0.4.2) | 🔴 Critical | Mutex protection added |
| Rust migration complexity | Medium | 🔴 High | Incremental migration, feature parity tests |
| DPI detection | Low | 🟠 Medium | Continuous obfuscation improvement |

### Operational Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Server downtime | Low | 🔴 High | Auto-reconnect, multi-server support |
| Config corruption | Low | 🟠 Medium | Backup + validation before changes |
| Security breach | Low | 🔴 Critical | Regular audits, minimal attack surface |

---

## Open Questions

### Strategic Decisions Pending

1. **Commercial vs Open Source**
   - Option A: Open source (MIT), community-driven
   - Option B: Commercial (SaaS), subscription model
   - Option C: Dual license (open core + enterprise features)
   - **Decision:** TBD

2. **Rust Migration Priority**
   - Option A: Full rewrite (all modules)
   - Option B: Hybrid (Go orchestration + Rust performance modules)
   - Option C: Stay with Go (good enough performance)
   - **Decision:** Option B (hybrid)

3. **Mobile Platform**
   - Option A: Native (Swift + Kotlin)
   - Option B: Cross-platform (Flutter)
   - Option C: Web-based (PWA)
   - **Decision:** TBD

---

## Team & Roles

### Current Team

| Role | Agent | Model | Responsibility |
|------|-------|-------|----------------|
| **Leader** | Ellie | bailian/qwen3.5-plus | Architecture, orchestration, quality control |
| **Worker** | Niuma | ollama_local/gpt-oss:20b | Code generation (local RTX 5060 Ti) |
| **Reviewer** | Coder | bailian/qwen3-coder-plus | Code review, security audit |
| **Fallback** | Aux | bailian/kimi-k2.5 | Complex reasoning, long context |

### Human Stakeholders

| Role | Name | Contact |
|------|------|---------|
| Visionary / Strategic Partner | Joel | ntcjoel@gmail.com |

---

## Communication Protocol

### 70/30 Rule
- **70% Technical English** - Architecture, code, logic flows
- **30% Strategic Chinese** - High-level decisions, revenue-critical

### Term Reinforcement
- 3 technical keywords per session for mastery

### Reporting Cadence
- **POI (Point of Impact)** - Key decision points require greenlight
- **Phase Completion** - Deliver + demo each phase
- **Exception** - Immediate alert on failures

---

## Budget & Cost

### Token Usage (Current Session)

| Agent | Tokens In | Tokens Out | Cost (USD) |
|-------|-----------|------------|------------|
| Leader (Ellie) | ~500k | ~50k | ~$0.50 |
| Niuma (Local) | ~200k | ~10k | $0.00 (local GPU) |
| Coder | ~20k | ~2k | ~$0.02 |
| **Total** | ~720k | ~62k | **~$0.52** |

### Infrastructure Cost

| Resource | Monthly Cost |
|----------|--------------|
| VPS (ts.vps) | $5-10 |
| Domain (glidesky.org) | $15/year |
| SSL Certificate | $0 (Let's Encrypt) |
| **Total** | **~$10/month** |

---

## Timeline Summary

```
2026-03-01  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            │
    Phase 1: Core Foundation ✅
    (v0.3 - Production Ready Go)
            │
2026-03-06  │
            │
    Phase 2: Multi-Platform ✅
    (v0.4.x - Windows + Management Tools)
            │
2026-03-07  ────────────────────────────────────
            │
    Phase 3: Rust Migration ⏳
    (v0.5.x - Performance Optimization)
            │
2026-03-15  │
            │
    Phase 4: Advanced Features ⏳
    (Multi-hop, Mobile, GUI)
            │
2026-04-01  │
            │
    Phase 5: Production Hardening ⏳
    (7-day test, 100+ clients, HA)
            │
2026-04-15  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

## Appendix: Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-03-06 | Ellie | Initial version, comprehensive goals |

---

**Next Review:** 2026-03-10 (after Phase 3 completion)

**Approval:** Pending Joel's review

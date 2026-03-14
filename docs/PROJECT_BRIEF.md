# WSVPN Project Brief - V0.1

## Overview
Develop a user-space VPN that bypasses traditional restrictions using HTTPS WebSocket encryption. The traffic should appear as normal HTTPS without TLS-in-TLS characteristics.

## Technical Requirements

### Core Architecture
- **Transport**: WebSocket over HTTPS (single TLS layer)
- **Encryption**: WebSocket payload encryption (no double TLS)
- **DPI Evasion**: Traffic must look like normal HTTPS web traffic
- **User-space**: No kernel modules required

### Network Configuration
- **VPN Name**: wn
- **Default Network**: 10.9.1.0/24
- **Server IP**: 10.9.1.1
- **Client IP**: 10.9.1.2 (test client)

### Test Environment
```
Test Client (tc.vps):
  SSH: ssh sq@10.1.1.6 -p2200
  User: sq / Password: sq5203281l
  Root: wq520sq1

Test Server (ts.vps):
  SSH: ssh sq@10.1.1.6
  User: sq / Password: sq5203281l
  Root: wq520sq1

Web Server: https://app.glidesky.org (running on 10.1.1.6)
```

### Success Criteria
1. Client 10.9.1.2 can ping server 10.9.1.1 through the VPN tunnel
2. Traffic appears as normal HTTPS to DPI inspection
3. No TLS-in-TLS characteristics detectable

## Agent Roles

### Leader (Ellie)
- Project research and architecture design
- Multi-agent coordination
- Prompt generation for sub-agents
- Nginx configuration for /{uuid} test endpoint
- Final testing and validation

### Agent niuma
- Code implementation
- WebSocket client/server development
- TUN/TAP interface handling
- Encryption layer implementation

### Agent Coder
- Documentation writing
- Technical specifications
- User guides
- Email updates at key milestones

## Development Phases

### Phase 1: Research & Design
- [ ] DPI evasion technique research
- [ ] WebSocket VPN architecture review
- [ ] Security model design
- [ ] Protocol specification

### Phase 2: Core Implementation
- [ ] WebSocket server (Go)
- [ ] WebSocket client (Go)
- [ ] TUN/TAP interface handler
- [ ] Payload encryption

### Phase 3: Testing & Validation
- [ ] Nginx /{uuid} endpoint setup
- [ ] Connectivity tests (ping 10.9.1.1 → 10.9.1.2)
- [ ] DPI simulation tests
- [ ] Performance benchmarks

### Phase 4: Documentation & Delivery
- [ ] User documentation
- [ ] Configuration guides
- [ ] Deployment scripts
- [ ] Final email report

## Communication Protocol
- Coder sends email updates at each phase completion
- Leader coordinates all agent activities
- All code must be complete (no partial snippets)
- Failure analysis required before implementation

## Security Notes
- Never expose infrastructure details publicly
- Use UUID-based endpoints for obfuscation
- Single TLS layer only (terminate at WebSocket level)

# Lightwell demos

Terminal GIFs for presenting Lightwell access tokens. Source of truth is the `.tape` + helper script; regenerate assets when the API story changes.

## Access tokens (deny → allow → revoke)

| File | Role |
|------|------|
| [lightwell_tokens.tape](lightwell_tokens.tape) | Charmbracelet [VHS](https://github.com/charmbracelet/vhs) cassette |
| [lightwell_tokens.gif](lightwell_tokens.gif) | Rendered GIF (commit after regenerating) |
| [lightwell_tokens.mp4](lightwell_tokens.mp4) | Same recording as MP4 |
| [../../scripts/demo_lightwell_tokens.sh](../../scripts/demo_lightwell_tokens.sh) | Story-paced demo driver (outcomes, not HTTP dumps) |
| [../../scripts/render_lightwell_tokens_demo.sh](../../scripts/render_lightwell_tokens_demo.sh) | Writes env + runs `vhs` |

### What the GIF shows

1. **Org admin creates an access token** — token created; secret shown once (masked)  
2. **Access is denied for non-admin users and credential-less requests** — no credential, regular (non-admin) user, and forged token all fail  
3. **Access is allowed for org admins or valid tokens** — Bearer token and org-admin Console identity both succeed  
4. **Revoke closes the door** — after revoke, the same token fails  

This uses the Content Sources **packages API** (org-admin or Lightwell Bearer). It does **not** claim Pulp content URL guarding until that guard is attached (`EXPECT_PULP_GUARDED=1` in the smoke script).

### Prerequisites

1. Install VHS (macOS): `brew install vhs` (needs `ttyd` and `ffmpeg`)
2. Backend: `make run` with Lightwell enabled and `lightwell-network` available for the demo org
3. A seeded Lightwell repository UUID (`REPO_UUID`)

List Lightwell repos:

```bash
curl -s -H "$(./scripts/header_org_admin.sh 9999 1111)" \
  "http://localhost:8000/api/content-sources/v1.0/repositories/?origin=lightwell" \
  | jq '.data[] | {uuid, name}'
```

### Render

```bash
REPO_UUID=<uuid> ./scripts/render_lightwell_tokens_demo.sh
```

Optional env overrides: `CS_BASE`, `ORG_ID`, `USER_ID`, `STEP_PAUSE` (seconds between demo beats; default `2`).

Dry-run without VHS:

```bash
REPO_UUID=<uuid> ./scripts/demo_lightwell_tokens.sh
```

### Presenting (2–3 minutes)

1. **One sentence:** Org admins issue credentials; anonymous package access fails; revoke is instant.  
2. **Play the GIF** (loop once or twice) and narrate the four banners.  
3. **Optional slide:** Console **Access tokens** UI + “Copy this token now. It will not be shown again.” — same control plane; the GIF is what CI sees.  
4. **If asked about Maven / artifact URLs:** content downloads will call the same validate path once the Pulp content guard ships — see [lightwell_tokens_pulp_handoff.md](../lightwell_tokens_pulp_handoff.md).

### Rehearsal checklist

- [ ] `make run` healthy; `REPO_UUID` returns packages with org-admin identity  
- [ ] `./scripts/demo_lightwell_tokens.sh` prints all four beats with created / denied / allowed / revoked  
- [ ] GIF readable full-screen (bump `FontSize` / `STEP_PAUSE` in the tape or render env if beats flash by)  
- [ ] Do not demo `/internal/lightwell/tokens/validate` as a customer flow  
- [ ] Do not claim open Pulp content URLs are denied while still unguarded  

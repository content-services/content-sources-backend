# Pulp Service Account Setup

## Overview

The local Pulp environment now creates a **non-superuser service account** to mimic stage/prod behavior.

This ensures that bugs related to RBAC permissions (like the upload repository bug) are caught locally before deployment.

## Accounts Available

### Service Account (Default)
- **Username:** `content-sources-svc`
- **Password:** `service-password`
- **Superuser:** No ❌
- **Permissions:** 479 (all RPM, Core, Container permissions)
- **Purpose:** Mimics stage/prod behavior

### Admin Account (For debugging)
- **Username:** `admin`
- **Password:** `password`
- **Superuser:** Yes ✅
- **Purpose:** Administrative tasks, debugging

## How It Works

1. **Migration** - Database schema is created
2. **Set Admin Password** - Default admin user gets password
3. **Init Service Account** - (NEW) Creates `content-sources-svc` with:
   - Non-superuser status
   - All necessary permissions
   - Default domain creation

## Configuration

Default `config.yaml` now uses the service account:

```yaml
clients:
  pulp:
    username: content-sources-svc
    password: service-password
```

To switch to admin (for debugging):
```yaml
clients:
  pulp:
    username: admin
    password: password
```

## Testing the Fix

### Before the Fix
With service account, `CreatePackage` without `repository` parameter fails:
```
400 Bad Request: ["Destination upload repository was not provided."]
```

### After the Fix
With `repository` parameter added, it works correctly.

## Customization

Environment variables in `docker-compose.yml`:
- `SERVICE_ACCOUNT_USERNAME` - Service account username (default: `content-sources-svc`)
- `SERVICE_ACCOUNT_PASSWORD` - Service account password (default: `service-password`)
- `DEFAULT_DOMAIN` - Default domain name (default: `default`)

## Files Modified

1. `assets/init_service_account.py` - Initialization script
2. `docker-compose.yml` - Added `init_service_account` service
3. `../../../configs/config.yaml.example` - Updated default credentials

## Troubleshooting

**Check service account creation:**
```bash
podman logs cs_init_service_account_1
```

**Verify account in Pulp:**
```bash
podman exec -it cs_pulp_api_1 bash
pulpcore-manager shell
>>> from django.contrib.auth import get_user_model
>>> user = get_user_model().objects.get(username='content-sources-svc')
>>> print(f"Superuser: {user.is_superuser}, Permissions: {user.user_permissions.count()}")
```

**Test permissions:**
```bash
curl -u content-sources-svc:service-password \
  https://localhost:8443/api/pulp/api/v3/users/ | jq
```

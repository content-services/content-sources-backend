#!/usr/bin/env python3
"""
Initialize a non-superuser service account for local development.
This mimics stage/prod behavior where the service uses a regular user, not admin.
"""
import os
import sys
import django

os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'pulpcore.app.settings')
django.setup()

from django.contrib.auth import get_user_model
from django.contrib.contenttypes.models import ContentType
from django.contrib.auth.models import Permission
from pulpcore.app.models import Domain

User = get_user_model()

# Service account credentials
SERVICE_USERNAME = os.environ.get('SERVICE_ACCOUNT_USERNAME', 'content-sources-svc')
SERVICE_PASSWORD = os.environ.get('SERVICE_ACCOUNT_PASSWORD', 'service-password')
DEFAULT_DOMAIN = os.environ.get('DEFAULT_DOMAIN', 'default')

print(f"=== Initializing Pulp Service Account ===")

# 1. Ensure default domain exists (must be created by admin/superuser)
print(f"Checking domain '{DEFAULT_DOMAIN}'...")
admin_user = User.objects.get(username='admin')
domain, created = Domain.objects.get_or_create(name=DEFAULT_DOMAIN)
if created:
    print(f"✓ Created domain: {DEFAULT_DOMAIN}")
else:
    print(f"✓ Domain already exists: {DEFAULT_DOMAIN}")

# 2. Create or update service account (non-superuser)
print(f"\nCreating service account '{SERVICE_USERNAME}'...")
service_user, created = User.objects.get_or_create(username=SERVICE_USERNAME)
service_user.set_password(SERVICE_PASSWORD)
service_user.is_superuser = False  # Important: NOT a superuser
service_user.is_staff = True       # Allow admin UI access for debugging
service_user.save()

if created:
    print(f"✓ Created user: {SERVICE_USERNAME}")
else:
    print(f"✓ Updated user: {SERVICE_USERNAME}")

print(f"  - is_superuser: {service_user.is_superuser}")
print(f"  - is_staff: {service_user.is_staff}")

# 3. Grant necessary permissions
print(f"\nGranting permissions to '{SERVICE_USERNAME}'...")

# Get all permissions for these app labels
permission_apps = ['rpm', 'core', 'container']
permissions = Permission.objects.filter(
    content_type__app_label__in=permission_apps
)

# Grant all permissions for these apps
service_user.user_permissions.set(permissions)
service_user.save()

print(f"✓ Granted {permissions.count()} permissions")
print(f"  - RPM permissions: {Permission.objects.filter(content_type__app_label='rpm').count()}")
print(f"  - Core permissions: {Permission.objects.filter(content_type__app_label='core').count()}")
print(f"  - Container permissions: {Permission.objects.filter(content_type__app_label='container').count()}")

print(f"\n=== Service Account Ready ===")
print(f"Username: {SERVICE_USERNAME}")
print(f"Password: {SERVICE_PASSWORD}")
print(f"Domain: {DEFAULT_DOMAIN}")
print(f"\nUpdate your config.yaml with:")
print(f"  pulp:")
print(f"    username: {SERVICE_USERNAME}")
print(f"    password: {SERVICE_PASSWORD}")

sys.exit(0)

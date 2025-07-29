#!/usr/bin/env python3

# Usage:
#
# 1. Set your username: `export USERNAME=<username>
# 2. Set your password: `export PASSWORD="<password>`"
# 3. OR set your Bearer token  BEARER_TOKEN="<my-token>"
# 4. Run: `python3 upload-rpm.py <repo_uuid> <rpm1 rpm2 ...>`

import os
import requests
import hashlib
import sys
import time
import subprocess
import re
import argparse
import tempfile
import re
import base64

base_url = 'https://console.redhat.com/api/content-sources/v1.0'

def default_headers():
    headers = {
              'Authorization': auth_header(),
              'Content-Type': 'application/json'
          }
    return headers

def auth_header():
    if (    'USERNAME' not in os.environ or 'PASSWORD' not in os.environ) and 'BEARER_TOKEN' not in os.environ:
        raise ValueError('Please set USERNAME and PASSWORD environment variables or BEARER_TOKEN environment variable')
    if 'BEARER_TOKEN' in os.environ:
        auth_header = "Bearer " + os.environ['BEARER_TOKEN']
    else:
        auth_string = os.environ['USERNAME'] + ":" + os.environ['PASSWORD']
        string_bytes = auth_string.encode('utf-8')
        b64_bytes = base64.b64encode(string_bytes)
        base64_string = b64_bytes.decode('utf-8')
        auth_header = "Basic " + base64_string
    return auth_header

def sanitize(input):
    if not re.match(r'^[a-zA-Z0-9/.\-_ ]+$', input) and not re.match(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', input):
        raise ValueError(f'Invalid input: {input}')
    return input

def split_rpm(rpm_file, chunk_name, chunk_size):
    # split the rpm into chunks
    subprocess.run(['split', '-b', f'{chunk_size}M', rpm_file, chunk_name])

def generate_checksum(rpm_file):
    # generate the checksum for the rpm
    sha256_hash = hashlib.sha256()
    rpm_file = sanitize(rpm_file)
    with open(rpm_file, 'rb') as f:
        for byte_block in iter(lambda: f.read(4096), b''):
            sha256_hash.update(byte_block)
    return sha256_hash.hexdigest()

def create_upload(size, chunk_size, rpm_sha256):
    # create the upload
    data = {'size': size, 'chunk_size': chunk_size, 'sha256': rpm_sha256}
    response = requests.post(f'{base_url}/repositories/uploads/', headers=default_headers(), json=data)
    response.raise_for_status()
    print("  upload uuid: " + response.json()['upload_uuid'])
    return response.json()['upload_uuid']


def upload_chunk(upload_id, file_path, total_size, start_byte, chunk_size, sha256):
    # upload a chunk
    with open(file_path, 'rb') as f:
        files = {'file': f}
        data = {'sha256': sha256}
        final_byte = start_byte + chunk_size - 1
        if final_byte > total_size-1:
            final_byte = total_size - 1

        headers = {
            'Content-Range': f'bytes {start_byte}-{final_byte}/*',
            'Authorization': auth_header(),
        }

        upload_uuid = sanitize(upload_id)

        response = requests.post(f'{base_url}/repositories/uploads/{upload_uuid}/upload_chunk/', headers=headers, files=files, json=data)
        response.raise_for_status()
        print(f'  uploaded chunk ({start_byte}-{final_byte})')
        return response.json()

def commit_upload(upload_href, sha256):
    # commit the upload
    data = {'sha256': sha256}
    headers = {
        'Authorization': auth_header(),
        'Content-Type': 'application/json'
    }
    upload_href = sanitize(upload_href)
    response = requests.post(f'{base_url}/pulp/uploads/{upload_href}', headers=headers, json=data)
    response.raise_for_status()
    return response.json()['task']

def get_artifact_href(task_href):
    # get either the artifact href or error from the task kicked off by the upload commit
    task_href = sanitize(task_href)
    while True:
        response = requests.get(f'{base_url}/pulp/tasks/{task_href}', headers=default_headers())
        response.raise_for_status()
        if not response.json()['state'] == 'completed' and not response.json()['state'] == 'failed':
            time.sleep(1)
            continue
        else:
            break
    if len(response.json()['created_resources']) == 0:
        raise ValueError(response.json()['error'])
    return response.json()['created_resources'][0]

def addArtifactsToRepository(repoUUID, artifacts):
    data = {
           'artifacts': []
    }
    for artifact in artifacts:
        data["artifacts"] += [{"href":artifact[1], "sha256": artifact[0]}]
    response = requests.post(f'{base_url}/repositories/{repoUUID}/add_uploads/', headers=default_headers(), json=data)
    response.raise_for_status()
    
def addUploadsToRepository(repoUUID, uploads):
   data = {
           'uploads': []
   }
   for upload in uploads:
     data["uploads"] += [{"uuid":upload[1], "sha256": upload[0]}]

   response = requests.post(f'{base_url}/repositories/{repoUUID}/add_uploads/', headers=default_headers(), json=data)
   response.raise_for_status()


def main():
    parser = argparse.ArgumentParser(
                        prog='uploads',
                        description='uploads rpms to content-sources-backend')
    parser.add_argument('repo_uuid')
    parser.add_argument('files', metavar='RPM', type=str, nargs='+',
                        help='One or more files to upload')
    args = parser.parse_args()

    rpms = args.files
    repo_uuid = args.repo_uuid

    UUID_REGEX = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"

    if not re.match(UUID_REGEX, repo_uuid):
        raise ValueError(repo_uuid + " is not a valid UUID")

    upload_ids = [] #tuples of [sha256, upload_href or upload_uuid]
    for rpm_file in rpms:
        print("Uploading: " + rpm_file)
        if not os.path.isfile(rpm_file) or not rpm_file.endswith('.rpm'):
            raise ValueError('File does not exist or is not an rpm')

        tempDir = tempfile.mkdtemp()

        chunk_name = 'test_chunk'
        chunk_size = 5 # MB
        rpm_size = os.path.getsize(rpm_file)
        # split the rpm into chunks
        split_rpm(rpm_file, os.path.join(tempDir, chunk_name), chunk_size)

        # generate the checksum for the rpm
        rpm_sha256 = generate_checksum(rpm_file)
        print(f'  sha256 for rpm: {rpm_sha256}')

        # create the upload
        print(f'  rpm_size: {rpm_size}')
        upload_id = create_upload(rpm_size, chunk_size, rpm_sha256)
        upload_ids += [(rpm_sha256, upload_id)]

        # upload the chunks
        start_byte = 0
        for chunk_file in sorted(os.listdir(tempDir)):
            if chunk_file.startswith(chunk_name):
                chunk_path = os.path.join(tempDir, chunk_file)
                chunk_size = os.path.getsize(chunk_path)
                chunk_sha256 = generate_checksum(chunk_path)
                upload_chunk(upload_id, chunk_path, rpm_size, start_byte, chunk_size, rpm_sha256)
                start_byte += chunk_size

        # remove chunk files if any exist
        for file_name in os.listdir(tempDir):
            os.remove(os.path.join(tempDir, file_name))

    addUploadsToRepository(repo_uuid, upload_ids)
    return

if __name__ == '__main__':
    main()

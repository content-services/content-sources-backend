# Usage:
#
# 1. Create and activate virtual environment: `cd scripts && python3 -m venv .venv && source .venv/bin/activate`
# 2. Install requests: `pip3 install requests`
# 3. Set your identity header: `export IDENTITY_HEADER=<identity_header>`
# 4. Run: `python3 uploads.py <repo_uuid> <rpm1 rpm2 ...> --api <public | internal>`

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

BASE_URL = 'http://localhost:8000/api/content-sources/v1.0'
IDENTITY_HEADER = os.environ['IDENTITY_HEADER']
API = 'public'

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

def create_upload(size):
    # create the upload
    data = {'size': size}
    headers = {
        'x-rh-identity': IDENTITY_HEADER,
        'Content-Type': 'application/json'
    }
    if API == 'public':
        response = requests.post(f'{BASE_URL}/repositories/uploads/', headers=headers, json=data)
        response.raise_for_status()
        print("upload: " + response.json()['upload_uuid'])
        return response.json()['upload_uuid']
    elif API == 'internal':
        response = requests.post(f'{BASE_URL}/pulp/uploads/', headers=headers, json=data)
        response.raise_for_status()
        print("upload: " + response.json()['pulp_href'])
        return response.json()['pulp_href']

def upload_chunk(upload_id, file_path, total_size, start_byte, chunk_size, sha256):
    # upload a chunk
    with open(file_path, 'rb') as f:
        files = {'file': f}
        data = {'sha256': sha256}
        final_byte = start_byte + chunk_size - 1
        if final_byte > total_size-1:
            final_byte = total_size - 1

        headers = {
            'x-rh-identity': IDENTITY_HEADER,
            'Content-Range': f'bytes {start_byte}-{final_byte}/*'
        }
        if API == 'public':
            upload_uuid = sanitize(upload_id)
            response = requests.post(f'{BASE_URL}/repositories/uploads/{upload_uuid}/upload_chunk/', headers=headers, files=files, json=data)
        if API == 'internal':
            upload_id = sanitize(upload_id)
            response = requests.put(f'{BASE_URL}/pulp/uploads/{upload_id}', headers=headers, files=files, json=data)
        response.raise_for_status()
        return response.json()

def commit_upload(upload_href, sha256):
    # commit the upload
    data = {'sha256': sha256}
    headers = {
        'x-rh-identity': IDENTITY_HEADER,
        'Content-Type': 'application/json'
    }
    upload_href = sanitize(upload_href)
    response = requests.post(f'{BASE_URL}/pulp/uploads/{upload_href}', headers=headers, json=data)
    response.raise_for_status()
    return response.json()['task']

def get_artifact_href(task_href):
    # get either the artifact href or error from the task kicked off by the upload commit
    headers = {
        'x-rh-identity': IDENTITY_HEADER,
        'Content-Type': 'application/json'
    }
    task_href = sanitize(task_href)
    while True:
        response = requests.get(f'{BASE_URL}/pulp/tasks/{task_href}', headers=headers)
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
    headers = {
        'x-rh-identity': IDENTITY_HEADER,
        'Content-Type': 'application/json'
    }
    data = {
           'artifacts': []
    }
    for artifact in artifacts:
        data["artifacts"] += [{"href":artifact[1], "sha256": artifact[0]}]
    response = requests.post(f'{BASE_URL}/repositories/{repoUUID}/add_uploads/', headers=headers, json=data)
    response.raise_for_status()
    
def addUploadsToRepository(repoUUID, uploads):
   headers = {
        'x-rh-identity': IDENTITY_HEADER,
        'Content-Type': 'application/json'
   }
   data = {
           'uploads': []
   }
   for upload in uploads:
      if API == 'internal':
         data["uploads"] += [{"href":upload[1], "sha256": upload[0]}]
      elif API == 'public':
         data["uploads"] += [{"uuid":upload[1], "sha256": upload[0]}]

   response = requests.post(f'{BASE_URL}/repositories/{repoUUID}/add_uploads/', headers=headers, json=data)
   response.raise_for_status()


def main():
    parser = argparse.ArgumentParser(
                        prog='uploads',
                        description='uploads rpms to content-sources-backend')
    parser.add_argument('repo_uuid')
    parser.add_argument('files', metavar='RPM', type=str, nargs='+',
                        help='One or more files to upload')
    parser.add_argument('-f', '--finalize',
                        action='store_true',
                        help='finalize uploads before saving them')
    parser.add_argument('--api', choices=['public', 'internal'], default='public',
                        help='specify whether to use the public or internal APIs (default: public)')
    args = parser.parse_args()

    rpms = args.files
    finalize = args.finalize
    repo_uuid = args.repo_uuid

    global API
    API = args.api

    UUID_REGEX = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"

    if not re.match(UUID_REGEX, repo_uuid):
        raise ValueError(repo_uuid + " is not a valid UUID")

    upload_ids = [] #tuples of [sha256, upload_href or upload_uuid]
    for rpm_file in rpms:
        if not os.path.isfile(rpm_file) or not rpm_file.endswith('.rpm'):
            raise ValueError('File does not exist or is not an rpm')

        tempDir = tempfile.mkdtemp()

        chunk_name = 'test_chunk'
        chunk_size = 6 # MB
        rpm_size = os.path.getsize(rpm_file)
        print(rpm_size)


        # split the rpm into chunks
        split_rpm(rpm_file, os.path.join(tempDir, chunk_name), chunk_size)

        # generate the checksum for the rpm
        rpm_sha256 = generate_checksum(rpm_file)
        print(f'sha256 for rpm: {rpm_sha256}')

        # create the upload
        upload_id = create_upload(rpm_size)
        print(f'upload href or uuid: {upload_id}')
        upload_ids += [(rpm_sha256, upload_id)]

        # upload the chunks
        start_byte = 0
        for chunk_file in sorted(os.listdir(tempDir)):
            if chunk_file.startswith(chunk_name):
                chunk_path = os.path.join(tempDir, chunk_file)
                chunk_size = os.path.getsize(chunk_path)
                chunk_sha256 = generate_checksum(chunk_path)
                upload_chunk(upload_id, chunk_path, rpm_size, start_byte, chunk_size, chunk_sha256)
                start_byte += chunk_size

        # remove chunk files if any exist
        for file_name in os.listdir(tempDir):
            os.remove(os.path.join(tempDir, file_name))


    # add the uploads directly and we are done
    if not finalize:
        addUploadsToRepository(repo_uuid, upload_ids)
        return

    # finalize the uploads and then add them

    # commit/finish the upload
    artifacts = [] #tuples of sha256, artifact_href
    for upload in upload_ids:
        sha256 = upload[0]
        task_href = commit_upload(upload[1], sha256)
        print(f'task href: {task_href}')

        # retrieve the artifact href or error from the task
        artifact_href = get_artifact_href(task_href)
        print(f'artifact href or error: {artifact_href}')
        artifacts += [(sha256, artifact_href)]


    addArtifactsToRepository(repo_uuid, artifacts)

if __name__ == '__main__':
    main()

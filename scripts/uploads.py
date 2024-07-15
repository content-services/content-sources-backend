# Usage:
#
# 1. Create and activate virtual environment: `cd scripts && python3 -m venv .venv && source .venv/bin/activate`
# 2. Install requests: `pip3 install requests`
# 3. Set your identity header: `export IDENTITY_HEADER=<identity_header>`
# 4. Run: `python3 uploads.py <path_to_rpm>`

import os
import requests
import hashlib
import sys
import time
import subprocess
import re

BASE_URL = 'http://localhost:8000/api/content-sources/v1.0/pulp'
IDENTITY_HEADER = os.environ['IDENTITY_HEADER']

def sanitize(input):
    if not re.match(r'^[a-zA-Z0-9/.\-_ ]+$', input):
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
    response = requests.post(f'{BASE_URL}/uploads/', headers=headers, json=data)
    response.raise_for_status()
    return response.json()['pulp_href']

def upload_chunk(upload_href, file_path, total_size, start_byte, chunk_size, sha256):
    # upload a chunk
    with open(file_path, 'rb') as f:
        files = {'file': f}
        data = {'sha256': sha256}
        headers = {
            'x-rh-identity': IDENTITY_HEADER,
            'Content-Range': f'bytes {start_byte}-{start_byte + chunk_size - 1}/*'
        }
        print(headers)
        upload_href = sanitize(upload_href)
        response = requests.put(f'{BASE_URL}/uploads/{upload_href}', headers=headers, files=files, json=data)
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
    response = requests.post(f'{BASE_URL}/uploads/{upload_href}', headers=headers, json=data)
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
        response = requests.get(f'{BASE_URL}/tasks/{task_href}', headers=headers)
        response.raise_for_status()
        if not response.json()['state'] == 'completed' and not response.json()['state'] == 'failed':
            time.sleep(1)
            continue
        else:
            break
    if len(response.json()['created_resources']) == 0:
        return response.json()['error']
    return response.json()['created_resources'][0]

def main():
    rpm_file = sys.argv[1]
    if not os.path.isfile(rpm_file) or not rpm_file.endswith('.rpm'):
        raise ValueError('File does not exist or is not an rpm')

    chunk_name = 'test_chunk'
    chunk_size = 6 # MB
    rpm_size = os.path.getsize(rpm_file)

    # split the rpm into chunks
    split_rpm(rpm_file, chunk_name, chunk_size)

    # generate the checksum for the rpm
    rpm_sha256 = generate_checksum(rpm_file)
    print(f'sha256 for rpm: {rpm_sha256}')

    # create the upload
    upload_href = create_upload(rpm_size)
    print(f'upload_href: {upload_href}')

    # upload the chunks
    start_byte = 0
    for chunk_file in sorted(os.listdir('.')):
        if chunk_file.startswith(chunk_name):
            chunk_path = os.path.join('.', chunk_file)
            chunk_size = os.path.getsize(chunk_path)
            chunk_sha256 = generate_checksum(chunk_file)
            upload_chunk(upload_href, chunk_path, rpm_size, start_byte, chunk_size, chunk_sha256)
            start_byte += chunk_size

    # commit/finish the upload
    task_href = commit_upload(upload_href, rpm_sha256)
    print(f'task href: {task_href}')

    # retrieve the artifact href or error from the task
    artifact_href = get_artifact_href(task_href)
    print(f'artifact href or error: {artifact_href}')

    # remove chunk files if any exist
    for file_name in os.listdir('.'):
        if file_name.startswith(chunk_name):
            os.remove(os.path.join('.', file_name))

if __name__ == '__main__':
    main()
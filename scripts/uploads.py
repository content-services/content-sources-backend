# Usage:
#
# 1. Create and activate virtual environment: `cd scripts && python3 -m venv .venv && source .venv/bin/activate`
# 2. Install requests: `pip3 install requests`
# 3. Set your identity header: `export IDENTITY_HEADER=<identity_header>`
# 4. Run: `python3 uploads.py <repo_uuid> <rpm1 rpm2 ...> --api <public | internal>`
#
# Debugging options:
#
#   --split 4              split each rpm into 4 chunks, working out the chunk size
#   --chunk-size 1M        or set the chunk size yourself (default 6M)
#   --chunks 0,1,1,2       upload an explicit sequence of chunks, by index. Ranges
#                          ("0-3") and "all" are supported. Repeating an index
#                          uploads the same chunk twice.
#   --no-add               stop after uploading chunks, don't touch the repository
#   --resumable            set resumable=true when creating the upload (public API)
#   --keep-chunks          keep chunk files after upload (default: delete them)
#   -v                     print each API call as a curl command you can copy-paste
#
# Each run writes its chunks to a temp directory and prints the path. By default they
# are deleted after the upload completes; use --keep-chunks to inspect them afterwards.
#
# For example, to split an rpm into 4 chunks and upload the first two twice, to see
# what the API does, without adding anything to a repository:
#
#   python3 uploads.py <repo_uuid> foo.rpm --split 4 --chunks 0,0,1,1,2,3 --no-add -v

import argparse
import hashlib
import io
import json as jsonlib
import math
import os
import re
import shlex
import shutil
import sys
import tempfile
import time

import requests

BASE_URL = 'http://localhost:8000/api/content-sources/v1.0'
IDENTITY_HEADER = os.environ['IDENTITY_HEADER']
API = 'public'
VERBOSITY = False

# error messages in chunk summary are trimmed to this many characters
TRUNCATE_AT = 200

# how many bytes to flip when corrupting a chunk with --corrupt
CORRUPTION_BYTE_COUNT = 100


class UsageError(ValueError):
    # raised for bad input discovered once a file is open, reported as a usage
    # error rather than a traceback
    pass


def sanitize(input):
    if not re.match(r'^[a-zA-Z0-9/.\-_ ]+$', input) and not re.match(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', input):
        raise ValueError(f'Invalid input: {input}')
    return input


def parse_size(value):
    # parse a size such as "6M", "500K" or "1048576" into bytes
    match = re.match(r'^(\d+)([KMG]?)$', value.strip(), re.IGNORECASE)
    if not match:
        raise argparse.ArgumentTypeError(f'invalid size: {value} (expected e.g. 1048576, 500K or 6M)')
    multiplier = {'': 1, 'k': 1024, 'm': 1024 ** 2, 'g': 1024 ** 3}[match.group(2).lower()]
    size = int(match.group(1)) * multiplier
    if size <= 0:
        raise argparse.ArgumentTypeError('size must be greater than 0')
    return size


def positive_int(value):
    if not value.isdigit() or int(value) <= 0:
        raise argparse.ArgumentTypeError(f'invalid count: {value} (expected a positive integer)')
    return int(value)


def parse_chunk_spec(spec, total_chunks):
    # parse "0,1,1,2" or "0-3" or "all" into an ordered list of chunk indexes
    if spec is None or spec.strip().lower() == 'all':
        return list(range(total_chunks))

    indexes = []
    for part in spec.split(','):
        part = part.strip()
        if not part:
            continue
        range_match = re.match(r'^(\d+)-(\d+)$', part)
        if range_match:
            start, end = int(range_match.group(1)), int(range_match.group(2))
            if start > end:
                raise UsageError(f'invalid chunk range: {part}')
            indexes += list(range(start, end + 1))
        elif part.isdigit():
            indexes += [int(part)]
        else:
            raise UsageError(f'invalid chunk spec: {part}')

    for index in indexes:
        if index >= total_chunks:
            raise UsageError(f'chunk {index} is out of range, the file has {total_chunks} chunk(s) (0-{total_chunks - 1})')

    if not indexes:
        raise UsageError('no chunks selected')

    return indexes


def read_chunk(rpm_file, index, chunk_size, chunk_dir):
    # read a single chunk of the rpm and write it into the run's temp directory, so that
    # it can be inspected or re-sent by hand afterwards. Returns offset, contents and path
    rpm_file = sanitize(rpm_file)
    offset = index * chunk_size
    with open(rpm_file, 'rb') as f:
        f.seek(offset)
        data = f.read(chunk_size)

    path = os.path.join(chunk_dir, f'chunk.{index}')
    with open(path, 'wb') as f:
        f.write(data)

    return offset, data, path


def generate_checksum(rpm_file):
    # generate the checksum for the rpm
    sha256_hash = hashlib.sha256()
    rpm_file = sanitize(rpm_file)
    with open(rpm_file, 'rb') as f:
        for byte_block in iter(lambda: f.read(4096), b''):
            sha256_hash.update(byte_block)
    return sha256_hash.hexdigest()


def checksum_bytes(data):
    return hashlib.sha256(data).hexdigest()


def oneline(text):
    return ' '.join(text.split())


def truncate(text):
    if len(text) > TRUNCATE_AT:
        return text[:TRUNCATE_AT] + '...'
    return text


def curl_command(method, url, headers, json_body=None, form=None, curl_file=None):
    # build a copy-pasteable curl equivalent of a request
    parts = [f'curl -X {method} {shlex.quote(url)}']
    for key, value in headers.items():
        if key == 'x-rh-identity':
            parts += ['-H "x-rh-identity: $IDENTITY_HEADER"']
        else:
            parts += [f'-H {shlex.quote(f"{key}: {value}")}']
    if json_body is not None:
        parts += [f'-d {shlex.quote(jsonlib.dumps(json_body))}']
    if curl_file is not None:
        parts += [f'-F file=@{shlex.quote(curl_file)}']
    for key, value in (form or {}).items():
        parts += [f'-F {shlex.quote(f"{key}={value}")}']
    return (' \\\n  ').join(parts).split('\n')


def log_request(method, url, headers, body_summary, json_body, form, curl_file):
    if not VERBOSITY:
        return

    path = url[len(BASE_URL):] if url.startswith(BASE_URL) else url
    print(f'# {method} {path}')
    for line in curl_command(method, url, headers, json_body, form, curl_file):
        print(line)


def log_response(response, elapsed_ms):
    if not VERBOSITY:
        return

    body = response.text or ''
    print(f'# => {response.status_code} ({elapsed_ms:.0f}ms)')
    if body:
        try:
            body = jsonlib.dumps(response.json(), indent=2)
        except ValueError:
            pass
        print(body)
    print()


def api_request(method, path, json_body=None, form=None, file_part=None, extra_headers=None,
                expect_ok=True, curl_file=None):
    # every API call goes through here, so that --verbose can report on all of them
    url = f'{BASE_URL}{path}'

    headers = {'x-rh-identity': IDENTITY_HEADER}
    if json_body is not None:
        headers['Content-Type'] = 'application/json'
    if extra_headers:
        headers.update(extra_headers)

    files = None
    if file_part is not None:
        file_name, file_data = file_part
        files = {'file': (file_name, io.BytesIO(file_data))}

    if json_body is not None:
        body_summary = jsonlib.dumps(json_body)
    else:
        summary = []
        if file_part is not None:
            summary += [f'file=<{file_part[0]}: {len(file_part[1])} bytes>']
        summary += [f'{key}={value}' for key, value in (form or {}).items()]
        body_summary = ' '.join(summary)

    log_request(method, url, headers, body_summary, json_body, form, curl_file)

    start = time.monotonic()
    response = requests.request(method, url, headers=headers, json=json_body, data=form, files=files)
    log_response(response, (time.monotonic() - start) * 1000)

    if expect_ok:
        response.raise_for_status()
    return response


def create_upload(size, chunk_size, rpm_sha256, resumable):
    # create the upload
    data = {'size': size, 'chunk_size': chunk_size, 'sha256': rpm_sha256}
    if API == 'public':
        if resumable:
            data['resumable'] = True
        response = api_request('POST', '/repositories/uploads/', json_body=data)
        body = response.json()
        if body.get('artifact_href'):
            print(f'existing artifact: {body["artifact_href"]}')
        if body.get('completed_checksums'):
            print(f'completed checksums: {body["completed_checksums"]}')
        print("upload: " + body['upload_uuid'])
        return body['upload_uuid']
    elif API == 'internal':
        response = api_request('POST', '/pulp/uploads/', json_body=data)
        print("upload: " + response.json()['pulp_href'])
        return response.json()['pulp_href']


def upload_chunk(upload_id, chunk_index, offset, data, chunk_path, sha256, expect_ok=True, wrong_offset=None):
    # upload a chunk
    actual_offset = wrong_offset if wrong_offset is not None else offset
    final_byte = actual_offset + len(data) - 1
    headers = {'Content-Range': f'bytes {actual_offset}-{final_byte}/*'}
    form = {'sha256': sha256}
    file_part = (f'chunk.{chunk_index}', data)

    if wrong_offset is not None:
        print(f'!!! WRONG OFFSET for chunk {chunk_index}: using bytes {actual_offset}-{final_byte} instead of {offset}-{offset + len(data) - 1}')

    if API == 'public':
        upload_uuid = sanitize(upload_id)
        return api_request('POST', f'/repositories/uploads/{upload_uuid}/upload_chunk/',
                           form=form, file_part=file_part, extra_headers=headers,
                           expect_ok=expect_ok, curl_file=chunk_path)
    if API == 'internal':
        upload_id = sanitize(upload_id)
        return api_request('PUT', f'/pulp/uploads/{upload_id}',
                           form=form, file_part=file_part, extra_headers=headers,
                           expect_ok=expect_ok, curl_file=chunk_path)


def commit_upload(upload_href, sha256):
    # commit the upload
    data = {'sha256': sha256}
    upload_href = sanitize(upload_href)
    response = api_request('POST', f'/pulp/uploads/{upload_href}', json_body=data)
    return response.json()['task']


def get_artifact_href(task_href):
    # get either the artifact href or error from the task kicked off by the upload commit
    task_href = sanitize(task_href)
    while True:
        response = api_request('GET', f'/pulp/tasks/{task_href}')
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
    api_request('POST', f'/repositories/{repoUUID}/add_uploads/', json_body=data)


def addUploadsToRepository(repoUUID, uploads):
   data = {
           'uploads': []
   }
   for upload in uploads:
      if API == 'internal':
         data["uploads"] += [{"href":upload[1], "sha256": upload[0]}]
      elif API == 'public':
         data["uploads"] += [{"uuid":upload[1], "sha256": upload[0]}]

   api_request('POST', f'/repositories/{repoUUID}/add_uploads/', json_body=data)


def print_chunk_summary(results):
    print()
    print(f'{"idx":>4}  {"offset":>12}  {"len":>10}  {"sha256":10}  {"status":>6}  note')
    for result in results:
        print(f'{result["index"]:>4}  {result["offset"]:>12}  {result["length"]:>10}  '
              f'{result["sha256"][:8] + "..":10}  {result["status"]:>6}  {result["note"]}')
    print()


def chunk_size_for(rpm_size, chunk_size, split):
    # work out the chunk size, either from --chunk-size or by splitting the rpm --split ways
    if split is None:
        return chunk_size

    if rpm_size < split:
        raise UsageError(f'cannot split a {rpm_size} byte file into {split} chunks')

    size = math.ceil(rpm_size / split)

    # the count is ceil(size / chunk_size), which for very small files can skip over the
    # requested one: 5 bytes is 3 chunks at a chunk size of 2 and 5 chunks at 1, never 4
    actual = math.ceil(rpm_size / size)
    if actual != split:
        raise UsageError(f'cannot split a {rpm_size} byte file into {split} chunks, the closest is '
                         f'{actual}, use --chunk-size instead')

    return size


def corrupt_chunk(data, chunk_index, corrupt_chunks):
    # intentionally corrupt specific chunks for testing
    if corrupt_chunks and chunk_index in corrupt_chunks:
        corrupted = bytearray(data)
        # flip some bytes in the middle of the chunk
        mid = len(corrupted) // 2
        bytes_to_flip = min(CORRUPTION_BYTE_COUNT, len(corrupted) - mid)
        for i in range(bytes_to_flip):
            corrupted[mid + i] = (corrupted[mid + i] + 1) % 256
        print(f'!!! CORRUPTED chunk {chunk_index} (flipped {bytes_to_flip} bytes)')
        return bytes(corrupted)
    return data


def prepare_upload(rpm_file, chunk_size, split, resumable):
    # calculate sizes, generate checksum, create the upload, and prepare temp directory
    rpm_size = os.path.getsize(rpm_file)
    chunk_size = chunk_size_for(rpm_size, chunk_size, split)
    total_chunks = max(1, math.ceil(rpm_size / chunk_size))

    rpm_sha256 = generate_checksum(rpm_file)
    print(f'sha256 for rpm: {rpm_sha256}')

    print(f'rpm_size: {rpm_size}')
    print(f'chunk_size: {chunk_size} ({total_chunks} chunk(s))')
    upload_id = create_upload(rpm_size, chunk_size, rpm_sha256, resumable)
    print(f'upload href or uuid: {upload_id}')

    chunk_dir = tempfile.mkdtemp(prefix='uploads-')
    print(f'chunks: {chunk_dir}')

    return rpm_sha256, upload_id, chunk_dir, chunk_size, total_chunks


def upload_rpm(rpm_file, chunk_size, split, chunk_spec, resumable, debug_chunks, corrupt_chunks=None, wrong_offset_chunks=None):
    rpm_sha256, upload_id, chunk_dir, chunk_size, total_chunks = prepare_upload(
        rpm_file, chunk_size, split, resumable)

    chunk_indexes = parse_chunk_spec(chunk_spec, total_chunks)

    # convert wrong offset chunk positions to byte offsets
    wrong_offsets = None
    if wrong_offset_chunks:
        wrong_offsets = {}
        for chunk_idx, target_position in wrong_offset_chunks.items():
            if chunk_idx >= total_chunks:
                raise UsageError(f'wrong-offset chunk {chunk_idx} is out of range (file has {total_chunks} chunks: 0-{total_chunks-1})')
            if target_position >= total_chunks:
                raise UsageError(f'wrong-offset position {target_position} is out of range (file has {total_chunks} chunks: 0-{total_chunks-1})')
            target_offset = target_position * chunk_size
            wrong_offsets[chunk_idx] = target_offset

    # upload the chunks
    results = []
    attempted = set()
    succeeded = set()
    for chunk_index in chunk_indexes:
        offset, data, chunk_path = read_chunk(rpm_file, chunk_index, chunk_size, chunk_dir)

        # optionally corrupt the chunk data
        data = corrupt_chunk(data, chunk_index, corrupt_chunks)

        # save the potentially-corrupted chunk for inspection
        if corrupt_chunks and chunk_index in corrupt_chunks:
            corrupted_path = os.path.join(chunk_dir, f'chunk.{chunk_index}.corrupted')
            with open(corrupted_path, 'wb') as f:
                f.write(data)
            chunk_path = corrupted_path

        chunk_sha256 = checksum_bytes(data)
        # optionally use wrong offset for this chunk
        wrong_offset = wrong_offsets.get(chunk_index) if wrong_offsets else None
        # in chunk debugging mode a failing chunk is a result, not a reason to stop
        response = upload_chunk(upload_id, chunk_index, offset, data, chunk_path,
                                chunk_sha256, expect_ok=not debug_chunks, wrong_offset=wrong_offset)
        note = 'duplicate' if chunk_index in attempted else ''
        if response.ok:
            succeeded.add(chunk_index)
        else:
            note = (note + ' ' if note else '') + truncate(oneline(response.text))
        attempted.add(chunk_index)
        results += [{
            'index': chunk_index,
            'offset': offset,
            'length': len(data),
            'sha256': chunk_sha256,
            'status': response.status_code,
            'note': note,
        }]

    if debug_chunks or VERBOSITY:
        print_chunk_summary(results)

    failed = len([result for result in results if result['status'] >= 400])
    missing = sorted(set(range(total_chunks)) - succeeded)

    return rpm_sha256, upload_id, chunk_dir, failed, missing


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
    how_to_split = parser.add_mutually_exclusive_group()
    how_to_split.add_argument('--chunk-size', type=parse_size, default=6 * 1024 ** 2, metavar='SIZE',
                              help='size of each uploaded chunk, e.g. 500K or 6M (default: 6M)')
    how_to_split.add_argument('--split', type=positive_int, metavar='N',
                              help='split each rpm into N chunks, working out the chunk size for you')
    parser.add_argument('--chunks', metavar='SPEC',
                        help='upload only these chunk indexes, e.g. "0,1,1,2" or "0-3" or "all". '
                             'Repeat an index to upload the same chunk twice (default: all, in order)')
    parser.add_argument('--no-add', action='store_true',
                        help='stop after uploading the chunks, do not add anything to the repository')
    parser.add_argument('--resumable', action='store_true',
                        help='create the upload with resumable=true, reusing a matching upload if one exists')
    parser.add_argument('--corrupt', metavar='CHUNKS',
                        help='intentionally corrupt these chunk indexes, e.g. "0" or "1,2" (for testing)')
    parser.add_argument('--wrong-offset', metavar='MAPPING',
                        help='upload chunks at wrong positions, e.g. "1:2" uploads chunk 1 at chunk 2\'s position, or "0:1,1:0" swaps chunks 0 and 1')
    parser.add_argument('--keep-chunks', action='store_true',
                        help='keep chunk files after upload for inspection (default: delete them)')
    parser.add_argument('-v', '--verbose', action='store_true',
                        help='print each API call with headers, response, and a curl command to replay it')
    args = parser.parse_args()

    rpms = args.files
    finalize = args.finalize
    repo_uuid = args.repo_uuid

    global API
    API = args.api

    global VERBOSITY
    VERBOSITY = args.verbose

    if args.resumable and API == 'internal':
        parser.error('--resumable is only supported by the public API')

    chunk_size = args.chunk_size

    UUID_REGEX = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"

    if not re.match(UUID_REGEX, repo_uuid):
        raise ValueError(repo_uuid + " is not a valid UUID")

    # parse --corrupt flag
    corrupt_chunks = None
    if args.corrupt:
        corrupt_chunks = set()
        for part in args.corrupt.split(','):
            part = part.strip()
            if part.isdigit():
                corrupt_chunks.add(int(part))
            else:
                parser.error(f'invalid corrupt chunk index: {part}')

    # parse --wrong-offset flag (chunk positions, will be converted to byte offsets later)
    wrong_offset_chunks = None
    if args.wrong_offset:
        wrong_offset_chunks = {}
        for mapping in args.wrong_offset.split(','):
            mapping = mapping.strip()
            if ':' not in mapping:
                parser.error(f'invalid wrong-offset mapping: {mapping} (expected format: chunk:position)')
            chunk_str, position_str = mapping.split(':', 1)
            if not chunk_str.isdigit() or not position_str.isdigit():
                parser.error(f'invalid wrong-offset mapping: {mapping} (both chunk and position must be integers)')
            wrong_offset_chunks[int(chunk_str)] = int(position_str)

    upload_ids = [] #tuples of [sha256, upload_href or upload_uuid]
    chunk_dirs = []
    failures = 0
    incomplete = False
    for rpm_file in rpms:
        if not os.path.isfile(rpm_file) or not rpm_file.endswith('.rpm'):
            raise ValueError('File does not exist or is not an rpm')

        try:
            rpm_sha256, upload_id, chunk_dir, failed, missing = upload_rpm(
                rpm_file, chunk_size, args.split, args.chunks, args.resumable,
                args.chunks is not None, corrupt_chunks, wrong_offset_chunks)
        except UsageError as err:
            parser.error(str(err))

        chunk_dirs.append(chunk_dir)

        report = []
        if failed:
            report += [f'{failed} chunk upload(s) failed']
        if missing:
            report += [f'{len(missing)} chunk(s) never uploaded: {missing}']
        if report:
            incomplete = True
            failures += failed
            print(f'{rpm_file}: ' + ', '.join(report))

        upload_ids += [(rpm_sha256, upload_id)]

    # clean up chunk directories unless --keep-chunks was set
    def cleanup_chunks():
        if not args.keep_chunks:
            for chunk_dir in chunk_dirs:
                shutil.rmtree(chunk_dir, ignore_errors=True)

    # an upload missing chunks can't be committed or added, the server would just reject it
    if incomplete:
        if not args.no_add:
            print('not touching the repository, the uploads above are incomplete')
        cleanup_chunks()
        return 1 if failures else 0

    if args.no_add and not finalize:
        cleanup_chunks()
        return 0

    # add the uploads directly and we are done
    if not finalize:
        addUploadsToRepository(repo_uuid, upload_ids)
        cleanup_chunks()
        return 0

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

    if args.no_add:
        cleanup_chunks()
        return 0

    addArtifactsToRepository(repo_uuid, artifacts)
    cleanup_chunks()
    return 0

if __name__ == '__main__':
    sys.exit(main())

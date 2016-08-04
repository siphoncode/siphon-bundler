
import os
import zipfile
import hashlib
import json
import time
from io import BytesIO

import requests


def get_hashes(bundler_url):
    resp = requests.get(bundler_url)
    return resp.json()['hashes']

def post_archive_with_listing(bundler_url, files):
    fp = BytesIO()
    listing = {}
    with zipfile.ZipFile(fp, 'w') as zf:
        for name, content in files.items():
            listing[name] = sha256_str(content)
            zf.writestr('diffs/' + name, content)
        zf.writestr('listing.json', json.dumps(listing))
    fp.seek(0)
    response = requests.post(bundler_url, data=fp, headers={
        'Accept-Encoding': 'gzip;q=0,deflate,sdch'
    })
    fp.close()
    return response

def post_archive(app_dir, bundler_url, server_hashes):
    """ Stolen from `siphon.cli.commands.push` """
    previous_dir = os.getcwd()
    try:
        os.chdir(app_dir)
        fp = BytesIO()
        with zipfile.ZipFile(fp, 'w') as zf:
            listing = {}
            for root, dirs, files in os.walk('.'):
                if root == '.':
                    prefix = ''
                elif root.startswith('./'):
                    prefix = root[2:] + '/'
                else:
                    raise RuntimeError('Unexpected root "%s"' % root)
                for fil in files:
                    local_path = root + '/' + fil
                    remote_path = prefix + fil
                    zip_path = 'diffs/' + remote_path
                    sha = sha256(local_path)
                    listing[remote_path] = sha
                    # Only write this file into diffs/ if we need to
                    if should_push(sha, remote_path, server_hashes):
                        zf.write(local_path, zip_path)
            # Write out the listing file JSON
            zf.writestr('listing.json', json.dumps(listing))

        fp.seek(0)
        response = requests.post(bundler_url, data=fp, stream=True, headers={
            'Accept-Encoding': 'gzip;q=0,deflate,sdch'
        })
        fp.close()
        if response.ok:
            for line in response.iter_lines():
                print(line.decode('utf-8'))
    finally:
        os.chdir(previous_dir)

def should_push(sha, remote_path, server_hashes):
    """ Stolen from `siphon.cli.commands.push` """
    remote_sha = server_hashes.get(remote_path)
    if remote_sha is None:
        return True # no need to generate local hash unless we have to
    return sha != remote_sha

def sha256(local_path):
    """ Stolen from `siphon.cli.commands.push` """
    hasher = hashlib.sha256()
    with open(local_path, 'rb') as fp:
        for chunk in iter(lambda: fp.read(4096), b''):
            hasher.update(chunk)
    return hasher.hexdigest()

def sha256_str(s):
    hasher = hashlib.sha256()
    hasher.update(s.encode('utf-8'))
    return hasher.hexdigest()

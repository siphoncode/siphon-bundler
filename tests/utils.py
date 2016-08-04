
import json
import base64
import socket
import signal
import time
import os
import unittest
import subprocess
from urllib.parse import quote


class BundlerTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        print('Starting the bundler...')
        cls._proc = subprocess.Popen(
            ['./bundler.sh'],
            cwd='../',
            preexec_fn=os.setsid,  # process group
        )
        try:
            print('Waiting for the bundler to startup...')
            n = 0
            while 1:
                if port_is_open(8000):
                    print('Port is open!')
                    break
                time.sleep(1)
                n += 1
                if n > 25:
                    break
            if cls._proc.poll() == 1:  # check it hasn't exited
                raise RuntimeError('Failed to start the bundler.')
        except:
            os.killpg(cls._proc.pid, signal.SIGTERM)  # kill the whole group
            raise

    @classmethod
    def tearDownClass(cls):
        os.killpg(cls._proc.pid, signal.SIGTERM)  # kill the whole group
        cls._proc.wait()

def port_is_open(port):
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    result = sock.connect_ex(('127.0.0.1', port))
    if result == 0:
        b = True
    else:
        b = False
    sock.close()
    return b

def make_handshake(obj):
    payload = json.dumps(obj).encode('utf8')
    signature = 'dummy-signature'
    s = base64.b64encode(payload).decode('ascii')
    return map(quote, (s, signature))

def make_development_handshake(action, user_id, app_id):
    return make_handshake({
        'action': action,
        'user_id': 'user_id',
        'app_id': app_id
    })

def make_production_handshake(action, submission_id, app_id):
    return make_handshake({
        'action': action,
        'submission_id': submission_id,
        'app_id': app_id
    })

def count_files(path):
    n = 0
    for a, b, c in os.walk(path):
        n += len(c)
    return n

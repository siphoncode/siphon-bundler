
import json
import requests

from utils import BundlerTestCase, make_development_handshake
from push_utils import get_hashes, post_archive

class TestHandshakeSecurity(BundlerTestCase):
    def _make_url(self, action, handshake_action, app_id):
        token, signature = make_development_handshake(handshake_action,
            'testuser', app_id)
        return 'http://localhost:8000/v1/%s/%s/?handshake_token=%s' \
            '&handshake_signature=%s' % (action, app_id, token, signature)

    def _pull(self, app_id, handshake_action):
        # Should be no hashes yet
        push_url = self._make_url('push', 'push', app_id)
        server_hashes = get_hashes(push_url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), 0)

        # Push our test app files
        app_files_dir = 'test-data/push-files'
        post_archive(app_files_dir, push_url, server_hashes)

        # Do a pull (send empty asset hashes so that it sends us everything)
        pull_url = self._make_url('pull', handshake_action, app_id)
        headers = {'content-type': 'application/json'}
        return requests.post(pull_url, headers=headers, data=json.dumps({
            'asset_hashes': {}
        }))

    def test_handshake_valid_action(self):
        resp = self._pull('my-handshake-test-1', 'pull')
        self.assertEqual(resp.status_code, 200)

    def test_handshake_invalid_action(self):
        resp = self._pull('my-handshake-test-2', 'push')
        self.assertEqual(resp.status_code, 401)

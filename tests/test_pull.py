
import io
import json
import unittest
import requests
import zipfile

from utils import BundlerTestCase, make_development_handshake
from push_utils import get_hashes, post_archive


class TestPull(BundlerTestCase):
    def _make_url(self, action, app_id):
        token, signature = make_development_handshake(action, 'testuser',
            app_id)
        return 'http://localhost:8000/v1/%s/%s/?handshake_token=%s' \
            '&handshake_signature=%s' % (action, app_id, token, signature)

    def _pull(self, app_id, asset_hashes):
        # Should be no hashes yet
        push_url = self._make_url('push', app_id)
        server_hashes = get_hashes(push_url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), 0)

        # Push our test app files
        app_files_dir = 'test-data/push-files'
        post_archive(app_files_dir, push_url, server_hashes)

        # Do a pull (send empty asset hashes so that it sends us everything)
        pull_url = self._make_url('pull', app_id)
        headers = {'content-type': 'application/json'}
        return requests.post(pull_url, headers=headers, data=json.dumps({
            'asset_hashes': asset_hashes
        }))

    def test_pull(self):
        """ Emulates an initial pull where we want everything back. """
        app_id = 'test-app-for-pull'
        resp = self._pull(app_id, {})

        # Make sure the returned .zip file is the right size and contains
        # the files we're expecting.
        self.assertEqual(resp.status_code, 200)
        self.assertEqual(resp.headers['Content-Type'], 'application/zip')
        self.assertEqual(len(resp.content), 58202)

        with zipfile.ZipFile(io.BytesIO(resp.content)) as zf:
            names = zf.namelist()
            self.assertListEqual(names, ['__siphon_assets/images/landscape.png',
                'assets-listing', 'bundle-footer'])

    def test_pull__with_existing_assets(self):
        """ Emulates a pull where we already have an asset locally. """
        app_id = 'test-app-for-pull-assets'
        resp = self._pull(app_id, {
            'images/landscape.png': '5cef2535aa41f3ca2542ea3d7866e3fe441cd' \
                                    '3142c1d127666184cc5372d7153'
        })

        self.assertEqual(resp.status_code, 200)
        self.assertEqual(resp.headers['Content-Type'], 'application/zip')
        self.assertEqual(len(resp.content), 324)

        with zipfile.ZipFile(io.BytesIO(resp.content)) as zf:
            names = zf.namelist()
            self.assertListEqual(names, ['assets-listing', 'bundle-footer'])

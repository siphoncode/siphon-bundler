
import requests
import json

from utils import BundlerTestCase, make_development_handshake
from push_utils import get_hashes, post_archive, post_archive_with_listing


class TestPush(BundlerTestCase):
    def _make_url(self, app_id):
        token, signature = make_development_handshake('push', 'testuser',
            app_id)
        return 'http://localhost:8000/v1/push/%s/?handshake_token=%s' \
            '&handshake_signature=%s' % (app_id, token, signature)

    def test_push(self):
        app_id = 'test-push-app-id'
        bundler_url = self._make_url(app_id)

        # Should be no hashes yet
        server_hashes = get_hashes(bundler_url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), 0)

        # Push our test app files
        app_files_dir = 'test-data/push-files'
        post_archive(app_files_dir, bundler_url, server_hashes)

        # Make sure its storing all the hashes we expect
        server_hashes = get_hashes(bundler_url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), 5)

    def test_push__hidden_file_in_zip_archive(self):
        """
        The zip archive security mechanism should not block files starting
        with a period.
        """
        app_id = 'test-push-with-a-hidden-file'
        bundler_url = self._make_url(app_id)

        # Should be no hashes yet
        server_hashes = get_hashes(bundler_url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), 0)

        # Push our test app files
        app_files_dir = 'test-data/push-files-with-hidden'
        post_archive(app_files_dir, bundler_url, server_hashes)

        # Make sure its storing all the hashes we expect
        server_hashes = get_hashes(bundler_url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), 3)
        self.assertTrue('.hidden-file' in server_hashes)

    def test_push__zip_archive_security(self):
        """
        Protect against POST'd zip files with relative path names that
        could be used to maliciously overwrite a system file.
        """
        app_id = 'test-push-with-bad-zip'
        bundler_url = self._make_url(app_id)
        resp = post_archive_with_listing(bundler_url, {
            'valid-file': 'some-content',
            '../bad-file': 'some-content',
            'Siphonfile': '{"base_version": "0.3"}'
        })
        self.assertTrue('Internal error.' in str(resp.content))
        server_hashes = get_hashes(bundler_url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), 0)

    def test_push__renaming_a_file(self):
        """
        User reported an S3 GetKey() bug that was triggered when renaming
        a file. This tests for that.
        """
        app_id = 'test-push-rename-file'
        # Do a normal push and check that the hashes match
        bundler_url = self._make_url(app_id)
        resp = post_archive_with_listing(bundler_url, {
            'valid-file.png': 'some-content',
            'Siphonfile': '{"base_version": "0.3"}'
        })
        self.assertEqual(resp.status_code, 200)
        server_hashes = get_hashes(bundler_url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), 2)
        # Do another push but rename the file
        bundler_url = self._make_url(app_id)
        resp = post_archive_with_listing(bundler_url, {
            'valid-file-renamed.png': 'some-content',
            'Siphonfile': '{"base_version": "0.3"}'
        })
        self.assertEqual(resp.status_code, 200)
        server_hashes = get_hashes(bundler_url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), 2)
        self.assertTrue('valid-file-renamed.png' in server_hashes)
        self.assertTrue('valid-file.png' not in server_hashes)
        # Do a pull to trigger fetching from S3, which is where the bug
        # lurks.
        token, signature = make_development_handshake('pull', 'testuser',
            app_id)
        pull_url = 'http://localhost:8000/v1/pull/%s/?handshake_token=%s' \
            '&handshake_signature=%s' % (app_id, token, signature)
        headers = {'content-type': 'application/json'}
        resp = requests.post(pull_url, headers=headers, data=json.dumps({
            'asset_hashes': {}
        }))
        self.assertEqual(resp.status_code, 200)
        self.assertTrue('Internal error.' not in str(resp.content))

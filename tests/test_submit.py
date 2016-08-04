
import io
import json
import requests
import zipfile

from push_utils import get_hashes, post_archive
from utils import BundlerTestCase, make_production_handshake, \
    make_development_handshake, count_files

APP_FILES_DEFAULT = 'test-data/push-files'
APP_FILES_CHANGED = 'test-data/push-files-changed'


class TestSubmit(BundlerTestCase):
    def _make_submit_url(self, submission_id, app_id):
        token, signature = make_production_handshake('submit', submission_id,
            app_id)
        return 'http://localhost:8000/v1/submit/%s/?handshake_token=%s' \
            '&handshake_signature=%s' % (app_id, token, signature)

    def _push(self, app_id, app_files_dir, assert_no_files=True):
        token, signature = make_development_handshake('push', 'testuser',
            app_id)
        url = 'http://localhost:8000/v1/push/%s/?handshake_token=%s' \
            '&handshake_signature=%s' % (app_id, token, signature)
        server_hashes = get_hashes(url)
        self.assertTrue(isinstance(server_hashes, dict))
        if assert_no_files:
            self.assertEqual(len(server_hashes), 0)
        # Push our test files
        post_archive(app_files_dir, url, server_hashes)
        # Check it matches
        server_hashes = get_hashes(url)
        self.assertTrue(isinstance(server_hashes, dict))
        self.assertEqual(len(server_hashes), count_files(app_files_dir))

    def _pull_submission(self, submission_id, app_id):
        token, signature = make_production_handshake('pull', submission_id,
            app_id)
        pull_url = 'http://localhost:8000/v1/pull/%s/?handshake_token=%s' \
            '&handshake_signature=%s&submission_id=%s' % (
            app_id, token, signature, submission_id)

        headers = {'content-type': 'application/json'}
        return requests.post(pull_url, headers=headers, data=json.dumps({
            'asset_hashes': {}
        }))

    def _check_submission(self, submission_id, app_id):
        resp = self._pull_submission(submission_id, app_id)
        self.assertEqual(resp.status_code, 200)
        self.assertEqual(resp.headers['Content-Type'], 'application/zip')
        with zipfile.ZipFile(io.BytesIO(resp.content)) as zf:
            names = zf.namelist()
            self.assertListEqual(names, [
                '__siphon_assets/images/landscape.png',
                'assets-listing',
                'bundle-footer'
            ])

    def test_submit(self):
        app_id = 'submit-app-id-1'
        submission_id = 'submit-id-1'

        # Push our app files first
        self._push(app_id, APP_FILES_DEFAULT)

        # Then make the submission snapshot
        url = self._make_submit_url(submission_id, app_id)
        resp = requests.post(url, data={'submission_id': submission_id})
        self.assertEqual(resp.status_code, 200)

        # Do a production pull to check that the submit was OK.
        self._check_submission(submission_id, app_id)

    def test_submit__immutability(self):
        """
        Submission files should not change if the corresponding app is
        pushed to after it is submitted.
        """
        app_id = 'submit-app-id-2'
        submission_id = 'submit-id-2'

        # Push our standard app files
        self._push(app_id, APP_FILES_DEFAULT)

        # Then make the submission snapshot
        url = self._make_submit_url(submission_id, app_id)
        resp = requests.post(url, data={'submission_id': submission_id})
        self.assertEqual(resp.status_code, 200)

        # Get the .zip content for the submission
        resp = self._pull_submission(submission_id, app_id)
        content_before = resp.content

        # Now push slightly changed files
        self._push(app_id, APP_FILES_CHANGED, assert_no_files=False)

        # Get the .zip content for the submission again, it should not
        # have changed at all.
        resp = self._pull_submission(submission_id, app_id)
        content_after = resp.content
        self.assertEqual(len(content_before), len(content_after))

    def test_submit__unknown_app_id(self):
        """
        Should not be able to submit with an app ID that does not exist.
        """
        app_id = 'unknown-app-1'
        submission_id = 'unknown-submission-1'
        url = self._make_submit_url(submission_id, app_id)
        resp = requests.post(url, data={'submission_id': submission_id})
        s = resp.content.decode('utf-8')
        self.assertEqual(resp.status_code, 400)
        self.assertTrue('App ID does not exist.' in s)

    def test_submit__already_submitted(self):
        """
        Should not be able to /submit if the submission_id already
        exists in the postgres DB.
        """
        app_id = 'submit-app-id-3'
        submission_id = 'submit-id-3'

        # Push our standard app files
        self._push(app_id, APP_FILES_DEFAULT)

        # Then make the submission snapshot
        url = self._make_submit_url(submission_id, app_id)
        resp = requests.post(url, data={'submission_id': submission_id})
        self.assertEqual(resp.status_code, 200)

        # Then try to make another one with the same submission_id,
        # it should fail
        url = self._make_submit_url(submission_id, app_id)
        resp = requests.post(url, data={'submission_id': submission_id})
        s = resp.content.decode('utf-8')
        self.assertEqual(resp.status_code, 400)
        self.assertTrue('Submission ID already exists.' in s)

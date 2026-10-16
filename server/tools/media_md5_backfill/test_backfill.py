import hashlib
import unittest
from unittest.mock import patch
import backfill


class Response:
    status_code = 200
    headers = {'Content-Type': 'video/mp4'}
    def __enter__(self): return self
    def __exit__(self, *_): pass
    def iter_content(self, _): yield b'original-video'


class FingerprintTests(unittest.TestCase):
    def test_original_stream_digest(self):
        with patch.object(backfill, 'stream_request', return_value=Response()):
            result = backfill.fingerprint((1,'https://test/video.mp4','',14,'video'),1e9,1024)
        self.assertEqual(result, (1,hashlib.md5(b'original-video').hexdigest(),14,None))

    def test_size_mismatch_never_writes_digest(self):
        with patch.object(backfill, 'stream_request', return_value=Response()):
            result = backfill.fingerprint((1,'https://test/video.mp4','',99,'video'),1e9,1024)
        self.assertIsNone(result[1])
        self.assertEqual(result[3], 'size_mismatch')

    def test_error_response_never_hashes_error_page(self):
        response = Response()
        response.status_code = 403
        with patch.object(backfill, 'stream_request', return_value=response):
            result = backfill.fingerprint((1,'https://test/video.mp4','',0,'video'),1e9,1024)
        self.assertEqual(result[3], 'http_403')
        self.assertIsNone(result[1])

    def test_exception_audit_does_not_contain_url(self):
        with patch.object(backfill, 'stream_request', side_effect=ValueError('secret-signed-url')):
            result = backfill.fingerprint((1,'https://test/video.mp4','',0,'video'),1e9,1024)
        self.assertEqual(result[3], 'ValueError')
        self.assertNotIn('secret', str(result))

    def test_no_remote_original(self):
        self.assertEqual(backfill.fingerprint((1,'','local/missing',0,'image'),1e9,1024)[3], 'no_remote_original')


if __name__ == '__main__': unittest.main()

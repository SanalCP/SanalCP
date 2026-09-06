#!/usr/bin/env python3
import importlib.util
import os
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('bootstrap', Path(__file__).with_name('installer-bootstrap.py'))
bootstrap = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bootstrap)


class BootstrapTest(unittest.TestCase):
    def setUp(self):
        self.data = bootstrap.render('v0.9.61', 'a' * 40, 'b' * 64, '2026-09-06')

    def test_valid_template(self):
        bootstrap.validate(self.data)
        self.assertIn(b'export SANALCP_REF SANALCP_SHA256', self.data)
        self.assertIn(b'"$@"', self.data)

    def test_rejects_commands_wrong_source_and_missing_hash(self):
        for data in [self.data + b'echo evil\n', self.data.replace(b'githubusercontent.com', b'example.com'), self.data.replace(b'b' * 64, b'')]:
            with self.subTest(data=data), self.assertRaises(ValueError):
                bootstrap.validate(data)

    def test_rejects_bad_tag_and_oversized_payload(self):
        with self.assertRaises(ValueError):
            bootstrap.render('v0.9.61;id', 'a' * 40, 'b' * 64)
        with self.assertRaises(ValueError):
            bootstrap.validate(b'x' * 4097)

    def test_publish_preserves_permissions_and_backups(self):
        with tempfile.TemporaryDirectory() as temporary:
            target = Path(temporary) / 'kur'
            target.write_bytes(b'previous')
            target.chmod(0o640)
            backups = Path(temporary) / 'backups'
            bootstrap.publish(self.data, target, backups)
            self.assertEqual(target.read_bytes(), self.data)
            self.assertEqual(target.stat().st_mode & 0o777, 0o640)
            self.assertEqual(next(backups.iterdir()).read_bytes(), b'previous')
            self.assertFalse(list(Path(temporary).glob('.kur.*')))

    def test_invalid_payload_does_not_touch_live_file(self):
        with tempfile.TemporaryDirectory() as temporary:
            target = Path(temporary) / 'kur'
            target.write_bytes(b'previous')
            with self.assertRaises(ValueError):
                bootstrap.publish(b'bad', target, Path(temporary) / 'backups')
            self.assertEqual(target.read_bytes(), b'previous')

    def test_rejects_symlink_target(self):
        with tempfile.TemporaryDirectory() as temporary:
            real = Path(temporary) / 'real'
            real.write_bytes(b'previous')
            target = Path(temporary) / 'kur'
            target.symlink_to(real)
            with self.assertRaises(OSError):
                bootstrap.publish(self.data, target, Path(temporary) / 'backups')
            self.assertEqual(real.read_bytes(), b'previous')


if __name__ == '__main__':
    unittest.main()

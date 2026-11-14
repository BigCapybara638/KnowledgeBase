"""Exercise the real deploy script with a controlled external Docker CLI.

Run on Linux with Python 3 and Bash: python3 -m unittest discover -s tests/deploy -v
"""
import gzip
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class DeploymentTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue((ROOT / "deploy/deploy.sh").is_file(), "deployment script is missing")
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.server = self.root / "server"
        self.stage = self.root / "incoming"
        self.commands = self.root / "commands"
        for directory in (self.server, self.stage, self.commands):
            directory.mkdir()
        for name in ("data", "backups"):
            (self.server / name).mkdir()
        for name in ("deploy.sh", "compose.yaml", ".env.example"):
            shutil.copyfile(ROOT / "deploy" / name, self.stage / name)
        archive = gzip.compress(b"controlled Docker archive fixture")
        (self.stage / "knowledgebase-image.tar.gz").write_bytes(archive)
        (self.stage / "knowledgebase-image.tar.gz.sha256").write_text(
            hashlib.sha256(archive).hexdigest() + "  knowledgebase-image.tar.gz\n",
            encoding="utf-8", newline="\n",
        )
        self.env_bytes = b"# Keep this comment\nKB_PUBLIC_URL=https://wiki.axentris.ru\nCUSTOM_SECRET=a=b\n"
        (self.server / ".env").write_bytes(self.env_bytes)
        self.log = self.root / "docker.log"
        self.backup_image = self.root / "backup-image"
        docker = self.commands / "docker"
        docker.write_text("""#!/usr/bin/env bash
set -eu
printf '%s\\n' "$*" >> "$DEPLOY_TEST_LOG"
case "$*" in
  'image load --input '*) [ "${DEPLOY_TEST_FAILURE:-}" != load ] ;;
  'image inspect '*) [ "${DEPLOY_TEST_FAILURE:-}" != inspect ] ;;
  *'ps --status running -q knowledgebase-app')
    if [ "${DEPLOY_TEST_STOPPED:-}" != yes ]; then printf 'container-id\\n'; fi ;;
  *'backup')
    cat compose.override.yaml > "$DEPLOY_TEST_BACKUP_IMAGE"
    [ "${DEPLOY_TEST_FAILURE:-}" != backup ] ;;
  *'up -d --no-deps --wait --wait-timeout 120 knowledgebase-app')
    [ "${DEPLOY_TEST_FAILURE:-}" != ready ] ;;
esac
""", encoding="utf-8", newline="\n")
        docker.chmod(0o755)

    def run_deploy(self, failure="", stopped=False, image=None):
        env = os.environ.copy()
        env.update({
            "PATH": str(self.commands) + os.pathsep + env["PATH"],
            "DEPLOY_TEST_LOG": self.log.as_posix(),
            "DEPLOY_TEST_BACKUP_IMAGE": self.backup_image.as_posix(),
            "DEPLOY_TEST_FAILURE": failure,
            "DEPLOY_TEST_STOPPED": "yes" if stopped else "no",
        })
        bash = os.environ.get("DEPLOY_TEST_BASH", "bash")
        return subprocess.run(
            [bash, "--noprofile", "--norc", str(self.stage / "deploy.sh"),
             self.server.as_posix(), image or "knowledgebase:" + "a" * 40],
            env=env, capture_output=True, text=True, timeout=30,
        )

    def existing_installation(self):
        (self.server / "data/knowledgebase.db").write_bytes(b"existing database")
        (self.server / "backups/keep").write_bytes(b"existing backup")
        (self.server / "compose.yaml").write_text("old compose\n")
        (self.server / "compose.override.yaml").write_text("old image\n")

    def assert_preserved(self):
        self.assertEqual((self.server / ".env").read_bytes(), self.env_bytes)
        if (self.server / "data/knowledgebase.db").exists():
            self.assertEqual((self.server / "data/knowledgebase.db").read_bytes(), b"existing database")
            self.assertEqual((self.server / "backups/keep").read_bytes(), b"existing backup")

    def test_initial_install_preserves_env_and_waits_for_manual_bootstrap(self):
        result = self.run_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_preserved()
        self.assertIn("knowledgebase:" + "a" * 40,
                      (self.server / "compose.override.yaml").read_text())
        self.assertNotIn(" up ", self.log.read_text())
        self.assertIn("administrator", result.stdout)
        self.assertTrue((self.server / ".bootstrap-required").is_file())

    def test_interrupted_bootstrap_with_database_still_does_not_start_http(self):
        self.existing_installation()
        (self.server / ".bootstrap-required").touch()
        result = self.run_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("up -d", self.log.read_text())
        self.assertIn("administrator", result.stdout)
        self.assertTrue((self.server / ".bootstrap-required").is_file())
        self.assert_preserved()

    def test_update_backs_up_old_image_and_preserves_env_and_data(self):
        self.existing_installation()
        result = self.run_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_preserved()
        self.assertEqual(self.backup_image.read_text(), "old image\n")
        log = self.log.read_text()
        self.assertLess(log.index("image load"), log.index("backup"))
        self.assertLess(log.index("image inspect"), log.index("backup"))
        self.assertLess(log.index("backup"), log.index("up -d"))
        self.assertNotIn("pull ", log)
        self.assertIn("exec -T knowledgebase-app /app/kb backup", log)
        self.assertIn("exec -T knowledgebase-app /app/kb check", log)

    def test_stopped_service_is_backed_up_using_current_image(self):
        self.existing_installation()
        result = self.run_deploy(stopped=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.backup_image.read_text(), "old image\n")
        self.assertIn("run --rm --no-deps -T knowledgebase-app backup", self.log.read_text())
        self.assert_preserved()

    def test_failed_backup_does_not_install_new_configuration_or_start_service(self):
        self.existing_installation()
        result = self.run_deploy(failure="backup")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.server / "compose.yaml").read_text(), "old compose\n")
        self.assertEqual((self.server / "compose.override.yaml").read_text(), "old image\n")
        self.assertNotIn("up -d", self.log.read_text())
        self.assert_preserved()
        self.assertFalse((self.server / ".deploy-lock").exists())

    def test_failed_load_does_not_change_old_installation(self):
        self.existing_installation()
        result = self.run_deploy(failure="load")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.server / "compose.override.yaml").read_text(), "old image\n")
        self.assertNotIn("backup", self.log.read_text())
        self.assert_preserved()

    def test_archive_missing_expected_image_does_not_change_old_installation(self):
        self.existing_installation()
        result = self.run_deploy(failure="inspect")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.server / "compose.override.yaml").read_text(), "old image\n")
        self.assertNotIn("backup", self.log.read_text())
        self.assert_preserved()

    def test_corrupted_archive_is_rejected_before_docker(self):
        self.existing_installation()
        with (self.stage / "knowledgebase-image.tar.gz").open("ab") as archive:
            archive.write(b"corrupted in transit")
        result = self.run_deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.log.exists())
        self.assertEqual((self.server / "compose.override.yaml").read_text(), "old image\n")
        self.assert_preserved()

    def test_missing_archive_is_rejected_before_docker(self):
        (self.stage / "knowledgebase-image.tar.gz").unlink()
        result = self.run_deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.log.exists())
        self.assert_preserved()

    def test_readiness_failure_fails_deployment_without_automatic_downgrade(self):
        self.existing_installation()
        result = self.run_deploy(failure="ready")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("knowledgebase:", (self.server / "compose.override.yaml").read_text())
        self.assertEqual(self.log.read_text().count("up -d"), 1)
        self.assert_preserved()

    def test_missing_env_fails_before_docker_and_does_not_create_env(self):
        (self.server / ".env").unlink()
        result = self.run_deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.server / ".env").exists())
        self.assertFalse(self.log.exists())

    def test_invalid_image_is_rejected_before_docker(self):
        result = self.run_deploy(image="knowledgebase:tag;touch bad")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.log.exists())
        self.assert_preserved()

    def test_existing_deployment_lock_prevents_changes(self):
        (self.server / ".deploy-lock").mkdir()
        result = self.run_deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.log.exists())
        self.assertTrue((self.server / ".deploy-lock").is_dir())
        self.assert_preserved()


if __name__ == "__main__":
    unittest.main()

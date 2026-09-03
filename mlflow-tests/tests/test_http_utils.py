import os

import pytest

from tests.constants.config import Config
from tests.http_utils import configure_ca_bundle_environment


@pytest.fixture(scope="module", autouse=True)
def create_experiments_and_runs() -> dict:
    return {}


def test_configure_ca_bundle_environment(monkeypatch):
    ca_bundle = "/tmp/test-ca-bundle.crt"
    monkeypatch.setattr(Config, "CA_BUNDLE", ca_bundle)

    configure_ca_bundle_environment()

    for name in (
        "SSL_CERT_FILE",
        "REQUESTS_CA_BUNDLE",
        "CURL_CA_BUNDLE",
        "AWS_CA_BUNDLE",
    ):
        assert os.environ[name] == ca_bundle


def test_configure_ca_bundle_environment_unsets_empty_requests_bundles(monkeypatch):
    monkeypatch.setattr(Config, "CA_BUNDLE", "")
    monkeypatch.setenv("REQUESTS_CA_BUNDLE", "")
    monkeypatch.setenv("CURL_CA_BUNDLE", "")

    configure_ca_bundle_environment()

    assert "REQUESTS_CA_BUNDLE" not in os.environ
    assert "CURL_CA_BUNDLE" not in os.environ

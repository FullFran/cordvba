"""twin.eye_reads: the only place twin calls eye_client. Table-driven
against a stub client so no test performs a live HTTP request.
"""

from conftest import make_ica_record, make_metar_record

from twin.eye_reads import fetch_ica_records, fetch_latest_metar


class _StubEyeClient:
    def __init__(self, records_by_call):
        self._records_by_call = records_by_call
        self.calls = []

    def records(self, **kwargs):
        self.calls.append(kwargs)
        return self._records_by_call.pop(0)


def test_fetch_latest_metar_requests_the_newest_single_record():
    metar = make_metar_record()
    client = _StubEyeClient([[metar]])

    result = fetch_latest_metar(client)

    assert result is metar
    assert client.calls == [{"source": "metar-cordoba", "topic": "weather", "limit": 1}]


def test_fetch_latest_metar_returns_none_when_eye_has_nothing_yet():
    client = _StubEyeClient([[]])

    assert fetch_latest_metar(client) is None


def test_fetch_ica_records_requests_the_miteco_source():
    records = [make_ica_record(station="14021009", index=4)]
    client = _StubEyeClient([records])

    result = fetch_ica_records(client)

    assert result == records
    assert client.calls == [{"source": "miteco-ica", "topic": "air_quality", "limit": 500}]

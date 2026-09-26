# testdata

Recorded responses from public sources, used as test fixtures.

Tests never perform live HTTP requests. Public services are not our test
infrastructure, and a suite that fails because AEMET is having a bad morning
teaches nobody anything.

Layout: `testdata/<source-id>/<case>.<ext>`, matching the `id` in
`configs/sources.yaml`.

When recording a fixture, save the payload exactly as received — no
reformatting, no trimming. The point is to catch the day the shape changes.

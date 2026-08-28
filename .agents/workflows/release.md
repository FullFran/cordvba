# Workflow: Release — eye

## Steps

1. **Ensure main is green** — CI pipeline passes on the target commit.

2. **Determine version** — follow Semantic Versioning:
   - `MAJOR`: breaking API or config changes.
   - `MINOR`: new features, backward-compatible.
   - `PATCH`: bug fixes only.

3. **Tag**
   ```
   git tag -a v<MAJOR>.<MINOR>.<PATCH> -m "chore(release): v<MAJOR>.<MINOR>.<PATCH>"
   git push origin v<MAJOR>.<MINOR>.<PATCH>
   ```

4. **Build release binary** (CI handles this; for manual verification):
   ```
   make build
   # binary at bin/server, version stamped via ldflags
   ./bin/server --version  # or check logs for "version" field
   ```

5. **Build the release binaries**
   ```
   make build
   trivy image eye:dev
   ```
   No HIGH/CRITICAL CVEs before publishing.

6. **Publish image** — push to registry with the semver tag and `latest`.

7. **GitHub Release** — create via `gh release create v<version>` with a changelog entry listing commits since the last tag:
   ```
   gh release create v<version> --title "v<version>" --notes "$(git log v<prev>..HEAD --oneline)"
   ```

8. **Verify deployment** — hit `/health` on the deployed instance; confirm `version` field matches the tag.

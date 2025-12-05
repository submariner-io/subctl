#### Setting up Subctl Build in Konflux on New Branch

**Prerequisites:**

- Configuration added in konflux-ci/build-definitions repo
- Existing Konflux-configured branch to copy files from (e.g., `release-0.21`)

**Placeholders:**

- `<target-branch>`: Your target branch (e.g., `release-0.22`)
- `<X-Y>`: Version with dashes (e.g., `0-22`)

**Important:** Subctl uses gomod-only prefetch (NO RPM dependencies), similar to lighthouse but with a single root module.

##### 1. Checkout Bot's PR Branch

Bot creates PR on branch named `konflux-subctl-<X-Y>`.

```bash
git checkout konflux-subctl-<X-Y>
```

##### 2. Copy Tekton Configuration from Previous Release

```bash
TARGET_VERSION=$(echo "<target-branch>" | grep -oP '(?<=release-0\.)\d+$')
[ -z "$TARGET_VERSION" ] && { echo "ERROR: Invalid branch format"; exit 1; }
PREV_VERSION=$((TARGET_VERSION - 1))

for type in pull-request push; do
  git show "origin/release-0.${PREV_VERSION}:.tekton/subctl-0-${PREV_VERSION}-${type}.yaml" | \
    sed "s/0-${PREV_VERSION}/0-${TARGET_VERSION}/g; s/release-0\.${PREV_VERSION}/release-0.${TARGET_VERSION}/g" \
    > ".tekton/subctl-0-${TARGET_VERSION}-${type}.yaml"
done
```

**Note:** Extracts YAML from previous release and updates versions in one step. Avoids intermediate files and sed parameter insertion bugs.

##### 3. Add Konflux Dockerfile

```bash
git checkout origin/release-0.${PREV_VERSION} -- package/Dockerfile.subctl.konflux
sed -i "s/release-0.${PREV_VERSION}/release-0.${TARGET_VERSION}/g" package/Dockerfile.subctl.konflux
```

**Note:** Subctl Dockerfiles have no CPE labels (only BASE_BRANCH update needed).

##### 4. Commit Changes

```bash
git add .tekton/subctl-0-${TARGET_VERSION}-*.yaml package/Dockerfile.subctl.konflux
git commit -s -m "Add Konflux config for subctl"
```

##### 5. Update Tekton Task References

```bash
bash << 'EOF'
set -e

PATCHER_SHA="b001763bb1cd0286a894cfb570fe12dd7f4504bd"
EXPECTED_SHA256="080ad5d7cf7d0cee732a774b7e4dda0e2ccf26b58e08a8516a3b812bc73beb53"

SCRIPT=$(curl -sL "https://raw.githubusercontent.com/simonbaird/konflux-pipeline-patcher/${PATCHER_SHA}/pipeline-patcher")
ACTUAL_SHA256=$(echo "$SCRIPT" | sha256sum | cut -d' ' -f1)

if [[ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]]; then
  echo "ERROR: Script checksum mismatch!"
  exit 1
fi

echo "$SCRIPT" | bash -s bump-task-refs
EOF
git diff --quiet .tekton/*.yaml || \
  { git add .tekton/*.yaml && \
    git commit -s -m "Update Tekton task references to latest versions"; }
```

##### 6. Review and Push

```bash
git log origin/<target-branch>..HEAD
git status
git push origin konflux-subctl-<X-Y> --force-with-lease
```

Expected: 3 commits (bot's initial + configuration + task refs update), clean working tree.

**Troubleshooting:**

- **Commit message too long**: Use exactly "Add Konflux config for subctl" (32 chars).
- **RPM lockfiles**: Subctl has NO RPM dependencies. Do not create `.rpm-lockfiles` directory.

# Security scanning

All scans run via `docker-compose.security.yml` against podman-compose. No interactive auth.

## Bring up SonarQube (persistent)

    podman-compose -f docker-compose.security.yml up -d sonarqube sonarqube-db
    # wait for http://localhost:9000/api/system/status == UP

## One-shot scanners

    mkdir -p coverage
    podman-compose -f docker-compose.security.yml run --rm gosec
    podman-compose -f docker-compose.security.yml run --rm govulncheck
    podman-compose -f docker-compose.security.yml run --rm gitleaks
    podman-compose -f docker-compose.security.yml run --rm trivy-fs
    podman-compose -f docker-compose.security.yml run --rm semgrep
    podman-compose -f docker-compose.security.yml run --rm syft
    SNYK_TOKEN=<env> podman-compose -f docker-compose.security.yml run --rm snyk

Reports land in `coverage/` (gosec.json, govulncheck.txt, gitleaks.json, trivy.json, semgrep.json, sbom.cdx.json, snyk.json).

## SonarQube scan

    podman run --rm -v "$PWD":/usr/src sonarsource/sonar-scanner-cli \
      -Dsonar.host.url=http://localhost:9000 \
      -Dsonar.login=$SONAR_TOKEN

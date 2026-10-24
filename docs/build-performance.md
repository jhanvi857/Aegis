# Build and Runtime Performance

This document records measurements for the lean CPU-only development stack. Run the
commands below from the repository root after Docker Desktop is running.

## Measurement commands

```powershell
docker image ls --format "{{.Repository}}:{{.Tag}}\t{{.Size}}"
Measure-Command { docker compose --progress plain --profile core build }
Measure-Command { docker compose --progress plain --profile core build }
docker compose --profile core up -d
docker stats --no-stream aegis-python-ml aegis-api-gateway aegis-postgres aegis-frontend
docker compose --profile core down
```

For cache validation, append a temporary comment to one Python source file and repeat the
core build, then do the same for one Go source file. Restore those temporary edits after
measuring. Do not use `--no-cache` for normal development: it intentionally re-downloads
dependencies and is much slower than the cached path.

## Results

Record results from the target 16 GB, CPU-only laptop here:

| Measurement | Before | After | Target |
|---|---:|---:|---:|
| Python serving image size | not captured before optimization | 1.42 GB | well under 1.5 GB |
| Cold core build | not captured before optimization | not completed; no-cache run intentionally stopped | n/a |
| Python one-line rebuild | not captured before optimization | 14.05 s | < 30 s |
| Go one-line rebuild | not captured before optimization | 20.45 s | < 30 s |
| Core idle RAM | not captured before optimization | 279.4 MiB total | suitable for 16 GB laptop |
| Core loaded RAM | not captured before optimization | not measured | suitable for 16 GB laptop |

Measured on an HP Laptop 15-fd1xxx with an Intel Core Ultra 5 125H (14 cores, 18 logical
processors) and 15.4 GB RAM. The original build log supplied for this optimization reported
approximately 3 GB of CUDA wheel downloads and a roughly 550-second pip layer; an original
image-size measurement was not available, so it is intentionally not fabricated here.

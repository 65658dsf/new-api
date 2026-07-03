$tag = git rev-parse --short HEAD
$image = "ningmeng123/new-api:$tag"
$tar = "ningmeng123-new-api-$tag.tar"

docker build `
  --build-arg BUN_REGISTRY=https://registry.npmmirror.com `
  --build-arg BUN_NETWORK_CONCURRENCY=2 `
  -t $image `
  -f Dockerfile .

if ($LASTEXITCODE -ne 0) {
  throw "Docker build failed"
}

docker save -o $tar $image

if ($LASTEXITCODE -ne 0) {
  throw "Docker save failed"
}

Write-Host "镜像已导出：$tar"
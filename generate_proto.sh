#!/bin/bash
set -euo pipefail
repository_directory="$(cd "$(dirname "$0")" && pwd)"
toolchain_gopath="$(go env GOPATH)"
export PATH="$toolchain_gopath/bin:$PATH"
software_development_kit_proto="$toolchain_gopath/pkg/mod/github.com/cosmos/cosmos-sdk@v0.55.0/proto"
workspace="$repository_directory/build/proto_workspace"

rm -rf "$workspace"
mkdir -p "$workspace/traderswift/network/v1" "$workspace/traderswift/validatorset/v1" "$workspace/cosmos/msg/v1"
cp "$repository_directory/proto/network.proto" "$workspace/traderswift/network/v1/"
cp "$repository_directory/proto/validatorset.proto" "$workspace/traderswift/validatorset/v1/"
cp "$software_development_kit_proto/cosmos/msg/v1/msg.proto" "$workspace/cosmos/msg/v1/"
chmod -R u+w "$workspace"
cat > "$workspace/buf.gen.yaml" <<'YAML'
version: v2
plugins:
  - local: protoc-gen-gocosmos
    out: generated
    opt:
      - plugins=grpc
      - paths=source_relative
YAML
cd "$workspace"
buf generate --path traderswift/network/v1/network.proto --path traderswift/validatorset/v1/validatorset.proto
cp generated/traderswift/network/v1/network.pb.go "$repository_directory/network/network.pb.go"
cp generated/traderswift/validatorset/v1/validatorset.pb.go "$repository_directory/validatorset/validatorset.pb.go"

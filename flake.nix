{
  description = "Reticulum-Go development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };

        go = pkgs.go_1_24;
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            go-task
            revive
            gosec
            gnumake
          ];

          shellHook = ''
            echo "Reticulum-Go development environment"
            echo "Go version: $(go version)"
            echo "Task version: $(task --version 2>/dev/null || echo 'not available')"
            echo "Revive version: $(revive --version 2>/dev/null || echo 'not available')"
            echo "Gosec version: $(gosec --version 2>/dev/null || echo 'not available')"
          '';
        };

        packages.default = pkgs.buildGoModule {
          pname = "reticulum-go";
          version = "dev";
          src = ./.;
          vendorHash = "";
          subPackages = [ "cmd/reticulum-go" ];
          ldflags = [ "-s" "-w" ];
          CGO_ENABLED = "0";
        };
      });
}


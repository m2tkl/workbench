{
  description = "Workbench development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
  };

  outputs =
    {
      nixpkgs,
    }:
    let
      mkShellFor =
        system:
        let
          pkgs = import nixpkgs {
            inherit system;
          };
        in
        pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
          ];

          shellHook = ''
            export GOPATH="$PWD/.gopath"
            export GOMODCACHE="$PWD/.gopath/pkg/mod"
            export PATH="$GOPATH/bin:$PATH"
          '';
        };
    in
    {
      devShells.aarch64-darwin.default = mkShellFor "aarch64-darwin";
      devShells.x86_64-darwin.default = mkShellFor "x86_64-darwin";
      devShells.aarch64-linux.default = mkShellFor "aarch64-linux";
      devShells.x86_64-linux.default = mkShellFor "x86_64-linux";
    };
}

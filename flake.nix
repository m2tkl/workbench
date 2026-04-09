{
  description = "Workbench development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-24.11";
  };

  outputs =
    {
      self,
      nixpkgs,
    }:
    let
      systems = [
        "aarch64-darwin"
        "x86_64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];

      forAllSystems = f:
        nixpkgs.lib.genAttrs systems (
          system:
          let
            pkgs = import nixpkgs {
              inherit system;
            };
          in
          f pkgs
        );

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
      devShells = {
        aarch64-darwin.default = mkShellFor "aarch64-darwin";
        x86_64-darwin.default = mkShellFor "x86_64-darwin";
        aarch64-linux.default = mkShellFor "aarch64-linux";
        x86_64-linux.default = mkShellFor "x86_64-linux";
      };

      packages = forAllSystems (
        pkgs:
        let
          workbench = pkgs.buildGoModule {
            pname = "workbench";
            version = "0.1.0";
            src = ./.;
            modRoot = ".";
            subPackages = [ "cmd/workbench" ];
            proxyVendor = true;
            vendorHash = "sha256-ecWyRBlT1NjXHq7JF30IEUAdBr4J6oGkT7ioKyN91tg=";
          };
        in
        {
          default = workbench;
          workbench = workbench;
        }
      );

      apps = forAllSystems (pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${pkgs.system}.workbench}/bin/workbench";
        };
        workbench = {
          type = "app";
          program = "${self.packages.${pkgs.system}.workbench}/bin/workbench";
        };
      });
    };
}

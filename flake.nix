{
  description = "xintray: a status indicator that lives in the tray";

  inputs.nixpkgs.url = "nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      supportedSystems =
        [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });
    in {
      packages = forAllSystems (system:
        let pkgs = nixpkgsFor.${system};

        in {
          xintray = with pkgs;
            buildGoModule rec {
              pname = "xintray";
              version = "v0.1.20";
              src = ./.;

              vendorHash =
                "sha256-dlrQoijLU7ivv/3IOwtJYqXLnfbl2KQljw6T8aYAOME=";
              proxyVendor = true;

              nativeBuildInputs = [ pkg-config copyDesktopItems ];
              buildInputs = [
                git
                glfw
                libGL
                libGLU
                openssh
                pkg-config
                xorg.libXcursor
                xorg.libXi
                xorg.libXinerama
                xorg.libXrandr
                xorg.libXxf86vm
                xorg.xinput
              ];

              desktopItems = [
                (makeDesktopItem {
                  name = "Xin Tray";
                  exec = pname;
                  icon = pname;
                  desktopName = pname;
                })
              ];

              postInstall = ''
                mkdir -p $out/share/
                cp -r icons $out/share/
              '';
            };
        });

      defaultPackage = forAllSystems (system: self.packages.${system}.xintray);
      devShells = forAllSystems (system:
        let pkgs = nixpkgsFor.${system};
        in {
          default = pkgs.mkShell {
            shellHook = ''
              PS1='\u@\h:\@; '
              nix run github:qbit/xin#flake-warn
              echo "Go `${pkgs.go}/bin/go version`"
            '';
            buildInputs = with pkgs; [
              git
              go
              gopls
              go-tools

              glfw
              pkg-config
              xorg.libXcursor
              xorg.libXi
              xorg.libXinerama
              xorg.libXrandr
              xorg.libXxf86vm
              xorg.xinput

              go-font
            ];
          };
        });
    };
}


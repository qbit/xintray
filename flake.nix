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
            buildGo120Module rec {
              pname = "xintray";
              version = "v0.1.13";
              src = ./.;

              vendorHash =
                "sha256-mp9e0Ed2bvyyqbQgnmWhS1Tb341iW8bJdlxNcjTG4Vo=";
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
              echo "Go `${pkgs.go_1_20}/bin/go version`"
            '';
            buildInputs = with pkgs; [
              git
              go_1_20
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


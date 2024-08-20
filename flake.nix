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
              version = "v0.2.1";
              src = ./.;

              vendorHash = "sha256-5hb6nLx+5o65ziAXhuJpkSmseZNiCf16zduPRqiLLZ4=";
              proxyVendor = true;

              nativeBuildInputs = [ pkg-config copyDesktopItems ];
              buildInputs = [
                fyne
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

                wayland
                libxkbcommon
              ];

              buildPhase = ''
                ${fyne}/bin/fyne package
              '';

              installPhase = ''
                mkdir -p $out
                pkg="$PWD/xintray.tar.xz"
                cd $out
                tar --strip-components=1 -xvf $pkg
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
              fyne
              git
              go
              gopls
              go-tools
              nilaway

              glfw
              pkg-config
              xorg.libXcursor
              xorg.libXi
              xorg.libXinerama
              xorg.libXrandr
              xorg.libXxf86vm
              xorg.xinput

              libxkbcommon
              wayland
              
              go-font
            ];
          };
        });
    };
}


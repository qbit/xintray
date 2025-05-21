{
  description = "xintray: a status indicator that lives in the tray";

  inputs.nixpkgs.url = "nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      supportedSystems =
        [ "x86_64-linux" "x86_64-darwin" "aarch64-linux" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
          mkPkg = { pname, useWayland ? false, ... }@args:
            with pkgs;
            buildGoModule (args // {
              inherit pname;
              version = "v0.2.7";
              src = ./.;
              vendorHash = "sha256-tX/D/CJ+Fr+DkTKlHv4UxlRSwaBsOrr7buPp4Q3ypFU=";
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

              buildPhase =
                if useWayland then ''
                  ${fyne}/bin/fyne package --tags wayland
                '' else ''
                  ${fyne}/bin/fyne package
                '';

              installPhase = ''
                mkdir -p $out
                pkg="$PWD/xintray.tar.xz"
                cd $out
                tar --strip-components=1 -xvf $pkg
              '';
            });
        in
        {
          xintray = mkPkg {
            pname = "xintray";
            useWayland = true;
          };
          xintray-x11 = mkPkg {
            pname = "xintray-x11";
            useWayland = false;
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
              libGL
              libGLU
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


{
  description = "xintray: a status indicator that lives in the tray";

  inputs.nixpkgs.url = "nixpkgs/nixos-22.05";

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
          xintray = pkgs.buildGo118Module {
            pname = "xintray";
            version = "v0.0.0";
            src = ./.;

            vendorSha256 =
              "sha256-FQsILSY4xC2byrg7bMMTJ/HOuq7hMKIffsDYbfm+h6E=";
            proxyVendor = true;

            nativeBuildInputs = with pkgs; [ pkg-config ];
            buildInputs = with pkgs; [
              glfw
              libGL
              libGLU
              pkg-config
              xlibsWrapper
              xorg.libXcursor
              xorg.libXi
              xorg.libXinerama
              xorg.libXrandr
              xorg.libXxf86vm
              xorg.xinput
            ];
          };
        });

      defaultPackage = forAllSystems (system: self.packages.${system}.xintray);
      devShells = forAllSystems (system:
        let pkgs = nixpkgsFor.${system};
        in {
          default = pkgs.mkShell {
            shellHook = ''
              PS1='\u@\h:\@; '
              echo "Go `${pkgs.go}/bin/go version`"
            '';
            buildInputs = with pkgs; [
              git
              go
              gopls
              go-tools

              glfw
              pkg-config
              xlibsWrapper
              xorg.libXcursor
              xorg.libXi
              xorg.libXinerama
              xorg.libXrandr
              xorg.libXxf86vm
              xorg.xinput
            ];
          };
        });
    };
}


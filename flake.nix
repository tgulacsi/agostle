{
  description = "agostle";

  inputs = {
    # nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    nixpkgs.url = "github:nixos/nixpkgs/nixos-26.05";
    flake-utils.url = "github:numtide/flake-utils";
    nix2container.url = "github:nlewo/nix2container";
  };

  outputs =
    {
      self,
      nixpkgs,
      nix2container,
      flake-utils,
    }:
    let
      neededPkgs =
        pkgs:
        (with pkgs; [
          caladea
          carlito
          corefonts
          dejavu_fonts
          file
          gentium
          ghostscript
          graphicsmagick
          hunspell
          hunspellDicts.hu_HU
          iana-etc
          liberation_ttf
          libreoffice
          mupdf-headless
          # openimageio
          pdftk
          pkgs.perl5Packages.EmailOutlookMessage
          poppler-utils
          procps
          python3Packages.weasyprint
          qpdf
          takao
          unrtf
          wkhtmltopdf
        ]);
    in
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
        nix2containerPkgs = nix2container.packages.${system};
      in
      {
        devShells = {
          default = pkgs.mkShell {
            buildInputs =
              with pkgs;
              [
                go
              ]
              ++ (neededPkgs pkgs);
            shellHook = ''
              export CGO_ENABLED=0
              echo "Welcome to the Agostle devShell!"
            '';
          };
        };
        packages = {
          agostle = nix2containerPkgs.nix2container.buildImage {
            name = "agostle";
            config = {
              cmd = [
                "/app/bin/agostle"
                "serve"
                "0.0.0.0:9500"
              ];
            };
            copyToRoot =
              (with pkgs; [
                # dockerTools.binSh
                fakeNss
                # toybox
                cacert
                # dash
              ])
              ++ (neededPkgs pkgs);
          };

          dockerImage = pkgs.dockerTools.streamLayeredImage {
            name = "agostle";
            tag = "latest";
            maxLayers = 32;
            created = "now";

            contents =
              (with pkgs; [
                dockerTools.binSh
                fakeNss
                toybox
                cacert
                dash
              ])
              ++ (neededPkgs pkgs);

            config = {
              Cmd = [
                "/app/bin/agostle"
                "serve"
                "0.0.0.0:9500"
              ];
              ExposedPorts = {
                "9500/tcp" = { };
              };
              Volumes = {
                "/app/bin" = { };
              };
              WorkingDir = "/app";
            };

          };

        };
      }
    );
}

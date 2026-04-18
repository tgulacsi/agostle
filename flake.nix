{
  description = "agostle";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
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
        pkgs = import nixpkgs { inherit system; };
      in
      {
        packages = {
          dockerImage = pkgs.dockerTools.streamLayeredImage {
            name = "agostle";
            tag = "latest";
            maxLayers = 2;
            created = "now";

            # compressor = "zstd";

            # copyToRoot = pkgs.buildEnv {
            #   name = "image-root";
            #   created = "now";
            #   pathsToLink = [ "/bin" ];
            #   paths = with pkgs; [
            contents =
              with pkgs;
              [
                dockerTools.binSh
                fakeNss
                toybox
                cacert
                dash
              ]
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

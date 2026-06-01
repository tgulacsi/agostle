{
  description = "agostle";

  inputs = {
    # nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    nixpkgs.url = "github:nixos/nixpkgs/nixos-26.05";
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
        nixpkgs.config.allowUnfreePredicate =
          pkg: builtins.elem (nixpkgs.config.lib.getName pkg) [ "corefonts" ];

        packages = {
          dockerImage = pkgs.dockerTools.streamLayeredImage {
            name = "agostle";
            tag = "latest";
            maxLayers = 32;
            created = "now";

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

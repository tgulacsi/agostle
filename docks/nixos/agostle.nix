{
  pkgs ? import <nixpkgs> { },
}:
pkgs.dockerTools.streamLayeredImage {
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
  contents = with pkgs; [
    fakeNss
    dockerTools.binSh
    toybox

    cacert
    caladea
    carlito
    corefonts
    dash
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
  ];
  # };

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
}

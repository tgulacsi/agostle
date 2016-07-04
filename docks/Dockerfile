FROM debian:testing
MAINTAINER Tamás Gulácsi <tgulacsi78@gmail.com>

ENV DEBIAN_FRONTEND=noninteractive
RUN echo 'deb http://httpredir.debian.org/debian testing main contrib non-free' >/etc/apt/sources.list
RUN apt-get -y update && apt-get -y upgrade
RUN apt-get -y install fonts-sil-gentium fonts-dejavu-extra fonts-liberation fonts-takao-mincho ttf-mscorefonts-installer
RUN apt-get -y install \
	ghostscript graphicsmagick pdftk poppler-utils mupdf-tools \
	libemail-outlook-message-perl
RUN apt-get -y install libreoffice
RUN apt-get -y install wkhtmltopdf
RUN apt-get -y install runit

RUN adduser agostle

USER agostle
WORKDIR /home/agostle
EXPOSE 9500:9500
ENV LOGDIR=/home/agostle/log
ENTRYPOINT ["/bin/dash", "-c"]
CMD ["set -x; mkdir -p $LOGDIR; [ -e $LOGDIR/config ] || echo -e 's33554432\nt86400\nn0\n!gzip -9c -' >$LOGDIR/config; ./bin/agostle serve 0.0.0.0:9500 2>&1 | svlogd $LOGDIR"]

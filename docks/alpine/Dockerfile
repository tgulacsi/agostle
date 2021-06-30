#FROM golang:alpine as builder
#
#RUN apk -U upgrade
#RUN go install github.com/tgulacsi/agostle@latest
#
FROM alpine:latest
MAINTAINER Tamás Gulácsi <tgulacsi78@gmail.com>

#COPY --from=builder agostle /app/bin/agostle
RUN apk -U upgrade
RUN apk add ttf-dejavu ttf-liberation font-noto font-noto-emoji
RUN apk add msttcorefonts-installer 
RUN update-ms-fonts 
RUN fc-cache -f
# https://stackoverflow.com/questions/25193161/chfn-pam-system-error-intermittently-in-docker-hub-builds
RUN ln -sf /bin/true /usr/bin/chfn
# Missing:
# fonts-sil-gentium fonts-takao-mincho 
# pdftk 

RUN apk add \
        perl-email-mime-contenttype perl-email-address perl-email-address-xs perl-email-date-format perl-email-mime perl-email-mime-encodings perl-email-simple \
        perl-data-optlist perl-sub-exporter \
        perl-app-cpanminus make
RUN cpanm -i -f IO::All
RUN cpanm Email::Outlook::Message
RUN apk add ghostscript graphicsmagick poppler-utils mupdf-tools
RUN apk add libreoffice
RUN apk add wkhtmltopdf
RUN apk add procps

#RUN addgroup --quiet --gid 10507 agostle
#RUN adduser --quiet --gecos 'agostle' --disabled-password --uid 10507 --gid 10507 agostle

#USER agostle
#WORKDIR /home/agostle

WORKDIR /app
EXPOSE 9500:9500
VOLUME ["/app/bin"]
ENTRYPOINT ["/bin/sh", "-c"]
CMD ["/app/bin/agostle serve 0.0.0.0:9500"]

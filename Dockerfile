####################################################
# GOLANG BUILDER
####################################################
FROM golang:1.11 as go_builder

COPY . /go/src/github.com/malice-plugins/avast
WORKDIR /go/src/github.com/malice-plugins/avast
RUN go get -u github.com/golang/dep/cmd/dep && dep ensure
RUN go build -ldflags "-s -w -X main.Version=v$(cat VERSION) -X main.BuildTime=$(date -u +%Y%m%d)" -o /bin/avscan

####################################################
# PLUGIN BUILDER
####################################################
FROM ubuntu:xenial

LABEL maintainer "https://github.com/blacktop"

LABEL malice.plugin.repository = "https://github.com/malice-plugins/avast.git"
LABEL malice.plugin.category="av"
LABEL malice.plugin.mime="*"
LABEL malice.plugin.docker.engine="*"

# Create a malice user and group first so the IDs get set the same way, even as
# the rest of this may change over time.
RUN groupadd -r malice \
  && useradd --no-log-init -r -g malice malice \
  && mkdir /malware \
  && chown -R malice:malice /malware

# ENV AVAST_VERSION 2.1.2-1

# Install Avast AV
COPY license.avastlic /etc/avast/license.avastlic
RUN set -x \
  && echo "===> Install Avast..." \
  && echo 'deb http://deb.avast.com/lin/repo debian release' >> /etc/apt/sources.list \
  && apt-key adv --fetch-keys http://files.avast.com/files/resellers/linux/avast.gpg \
  && apt-get update -q && apt-get install -y avast-fss \
  && chown avast:avast /etc/avast/license.avastlic \
  # patch update script
  && download='DOWNLOAD="curl -L -s --connect-timeout 5 -f"' \
  && download_fix='DOWNLOAD="curl -L -s -f"' \
  && sed -i "s|$download|$download_fix|g" /var/lib/avast/Setup/avast.setup \
  && echo "===> Clean up unnecessary files..." \
  && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /root/.gnupg

# Ensure ca-certificates is installed for elasticsearch to use https
RUN apt-get update -qq && apt-get install -yq --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Add EICAR Test Virus File to malware folder
ADD http://www.eicar.org/download/eicar.com.txt /malware/EICAR

# Update Avast Definitions
RUN mkdir -p /opt/malice && echo "===> Update Avast..." && /var/lib/avast/Setup/avast.vpsupdate

COPY --from=go_builder /bin/avscan /bin/avscan

WORKDIR /malware

ENTRYPOINT ["/bin/avscan"]
CMD ["--help"]

####################################################
# NOTE: https://www.avast.com/en-us/faq.php?article=AVKB131
# NOTE: To Update run - /var/lib/avast/Setup/avast.vpsupdate

# http://files.avast.com/lin/repo/pool/avast_2.1.2-1_amd64.deb
# http://files.avast.com/lin/repo/pool/avast-fss_1.0.11-1_amd64.deb
# https://www.avast.com/registration-free-antivirus.php

# cpolar tunnel built from the official release (no third-party image).
FROM alpine:3.20
ARG CPOLAR_ARCH=arm64
RUN apk add --no-cache ca-certificates curl unzip \
 && curl -fsSL -o /tmp/cpolar.zip "https://www.cpolar.com/static/downloads/releases/latest/cpolar-stable-linux-${CPOLAR_ARCH}.zip" \
 && unzip -q /tmp/cpolar.zip -d /usr/local/bin && chmod +x /usr/local/bin/cpolar && rm /tmp/cpolar.zip
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["sh", "/entrypoint.sh"]

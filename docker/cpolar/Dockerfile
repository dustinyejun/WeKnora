# cpolar tunnel built from the official release (no third-party image).
FROM alpine:3.20
# BuildKit sets TARGETARCH from the platform being built (amd64, arm64, ...);
# cpolar's release zips use the same names. CPOLAR_ARCH only overrides odd cases.
ARG TARGETARCH
ARG CPOLAR_ARCH=${TARGETARCH}
RUN test -n "${CPOLAR_ARCH}" \
 || { echo "CPOLAR_ARCH is empty: build with BuildKit or pass --build-arg CPOLAR_ARCH=amd64|arm64" >&2; exit 1; } \
 && apk add --no-cache ca-certificates curl unzip \
 && curl -fsSL -o /tmp/cpolar.zip "https://www.cpolar.com/static/downloads/releases/latest/cpolar-stable-linux-${CPOLAR_ARCH}.zip" \
 && unzip -q /tmp/cpolar.zip -d /usr/local/bin && chmod +x /usr/local/bin/cpolar && rm /tmp/cpolar.zip
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["sh", "/entrypoint.sh"]

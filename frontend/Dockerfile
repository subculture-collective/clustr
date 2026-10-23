# Use a lightweight base image
# Build stage
FROM node:24-alpine AS build
WORKDIR /app
# Vite substitutes VITE_* values while compiling. Runtime environment passed
# to Nginx cannot change a shipped JavaScript bundle, so keep same-origin /api
# as the safe default and make a cross-origin target an explicit image input.
ARG VITE_API_URL=/api
ENV VITE_API_URL=${VITE_API_URL}
ARG VITE_CATALOG_ENABLED=false
ENV VITE_CATALOG_ENABLED=${VITE_CATALOG_ENABLED}
COPY package.json package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY . .
RUN npm run build

# Serve stage
FROM nginx:1.31.7-alpine@sha256:892bb2152464f0c5d8972593a5f5afaf8054552580b699d95f8ff4af08c2d33d
# Require fixes even when a prior upgrade layer exists in the build cache.
RUN apk upgrade --no-cache && \
    apk add --no-cache 'libexpat>=2.9.0-r0' 'libx11>=1.8.14-r0'
COPY --from=build /app/dist/ /usr/share/nginx/html/
COPY nginx.conf /etc/nginx/nginx.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:80/ || exit 1

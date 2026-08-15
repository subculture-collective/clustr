# Use a lightweight base image
# Build stage
FROM node:20-alpine AS build
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
FROM nginx:alpine
COPY --from=build /app/dist/ /usr/share/nginx/html/
COPY nginx.conf /etc/nginx/nginx.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

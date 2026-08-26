# Build the SPA, then serve the static bundle with nginx. The API base URL is baked at build time
# (Vite env), so the browser talks to the host-published gateway. schema.ts is committed, so the
# build needs no access to ../api.
FROM node:22-alpine AS build
WORKDIR /app
RUN corepack enable
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
ARG VITE_NEMPAY_API_URL=http://localhost:8080
ENV VITE_NEMPAY_API_URL=$VITE_NEMPAY_API_URL
RUN pnpm build

FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80

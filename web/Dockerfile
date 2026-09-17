# Production web image (opt-in daily driver — see the `web-prod` service in
# deploy/docker-compose.yml). The dev override runs `next dev` with hot reload,
# which compiles every route on demand (2-4s stalls per navigation on Windows
# bind mounts); this image serves an ahead-of-time `next build` instead.
# Build arg NEXT_PUBLIC_API_BASE is baked in (browser→API direct).
FROM node:22 AS deps
WORKDIR /web
COPY package.json pnpm-lock.yaml ./
# Registry bağlantısı dalgalı ortamlarda (sandbox/VPN) indirme yarıda kesilebiliyor;
# fetch retry bayrakları kurulumu dayanıklı kılar.
RUN corepack enable && pnpm install --frozen-lockfile \
  --fetch-retries=8 --fetch-retry-maxtimeout=180000 --fetch-timeout=90000

FROM node:22 AS build
WORKDIR /web
COPY --from=deps /web/node_modules ./node_modules
COPY . .
ARG NEXT_PUBLIC_API_BASE=http://localhost:8080/api/v1
ENV NEXT_PUBLIC_API_BASE=$NEXT_PUBLIC_API_BASE
RUN corepack enable && pnpm build

FROM node:22 AS run
WORKDIR /web
ENV NODE_ENV=production
COPY --from=build /web/package.json /web/pnpm-lock.yaml ./
COPY --from=build /web/node_modules ./node_modules
COPY --from=build /web/.next ./.next
COPY --from=build /web/next.config.mjs ./
EXPOSE 3000
CMD ["sh", "-c", "corepack enable && pnpm start --port 3000"]

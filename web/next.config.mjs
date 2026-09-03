/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // Keep container dev output separate from host production builds.
  distDir: process.env.NEXT_DIST_DIR ?? '.next',
  eslint: {
    // Lint is run explicitly via `pnpm lint`; don't fail production builds twice.
    ignoreDuringBuilds: false,
  },
};

export default nextConfig;

/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  eslint: {
    // Lint is run explicitly via `pnpm lint`; don't fail production builds twice.
    ignoreDuringBuilds: false,
  },
};

export default nextConfig;

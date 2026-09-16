/** @type {import('next').NextConfig} */
const nextConfig = {
  // The whole site is static: no server components that need a server, no API
  // routes, no image optimisation. `export` writes plain HTML into out/, which
  // Vercel serves as-is (and which any static host could).
  output: "export",
  images: { unoptimized: true },
  // Vercel's static hosting resolves /features → /features.html on its own;
  // keep the canonical URLs slash-less to match the old site's sitemap.
  trailingSlash: false,
  reactStrictMode: true,
};

export default nextConfig;

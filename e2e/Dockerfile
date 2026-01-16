# Playwright test runner with pre-installed dependencies
FROM mcr.microsoft.com/playwright:v1.57.0-jammy

WORKDIR /app/e2e

# Copy package files first for better layer caching
COPY package*.json ./

# Install dependencies during build (not at runtime)
RUN npm ci

# Copy test files
COPY . .

# Default command runs the tests
CMD ["npx", "playwright", "test", "--reporter=list"]

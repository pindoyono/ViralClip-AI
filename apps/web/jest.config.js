/** @type {import('jest').Config} */
const config = {
  testEnvironment: "jsdom",
  setupFilesAfterEnv: ["<rootDir>/jest.setup.js"],

  transform: {
    "^.+\\.(t|j)sx?$": ["@swc/jest", {
      jsc: {
        transform: {
          react: { runtime: "automatic" },
        },
      },
    }],
  },
  moduleNameMapper: {
    "^@/(.*)$": "<rootDir>/src/$1",
    "\\.(css|less|scss|sass)$": "identity-obj-proxy",
  },
  testMatch: [
    "**/__tests__/**/*.(spec|test).(ts|tsx|js|jsx)",
    "**/*.(spec|test).(ts|tsx|js|jsx)",
  ],
  collectCoverageFrom: [
    "src/**/*.(ts|tsx)",
    "!src/**/*.d.ts",
    "!src/app/layout.tsx",
    "!src/app/page.tsx",
  ],
};

module.exports = config;

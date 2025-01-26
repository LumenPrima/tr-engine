module.exports = {
  // Test environment
  testEnvironment: 'node',

  // Test file patterns
  testMatch: [
    '**/tests/**/*.test.js',
    '**/tests/**/*.spec.js'
  ],

  // Coverage configuration
  collectCoverageFrom: [
    'src/**/*.js',
    '!src/app.js',
    '!**/node_modules/**'
  ],
  coverageDirectory: 'coverage',
  coverageReporters: ['text', 'lcov', 'clover'],

  // Setup files
  setupFilesAfterEnv: ['./tests/setup.js'],

  // Test environment configuration
  testEnvironment: 'node',
  globalSetup: '<rootDir>/tests/globalSetup.js',

  // Timeouts
  testTimeout: 10000,

  // Verbose output
  verbose: true
};

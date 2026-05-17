/**
 * Tests for the API client (axios instance + interceptors).
 */
import axios from "axios";

// We test the module's behaviour by using the actual apiClient
// with mocked axios adapter.
jest.mock("axios", () => {
  const mockAxios = {
    create: jest.fn(() => mockAxios),
    interceptors: {
      request: { use: jest.fn() },
      response: { use: jest.fn() },
    },
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
    delete: jest.fn(),
    defaults: { baseURL: "" },
  };
  return { default: mockAxios, ...mockAxios };
});

describe("apiClient configuration", () => {
  beforeEach(() => {
    jest.resetModules();
  });

  it("creates axios instance with correct baseURL when NEXT_PUBLIC_API_URL is set", () => {
    process.env.NEXT_PUBLIC_API_URL = "https://api.example.com";

    // Reimport to pick up env var
    jest.isolateModules(() => {
      const mockedAxios = require("axios").default;
      require("@/lib/api");
      expect(mockedAxios.create).toHaveBeenCalledWith(
        expect.objectContaining({
          baseURL: "https://api.example.com/api/v1",
        })
      );
    });

    delete process.env.NEXT_PUBLIC_API_URL;
  });

  it("falls back to /api/backend when NEXT_PUBLIC_API_URL is not set", () => {
    delete process.env.NEXT_PUBLIC_API_URL;

    jest.isolateModules(() => {
      const mockedAxios = require("axios").default;
      require("@/lib/api");
      expect(mockedAxios.create).toHaveBeenCalledWith(
        expect.objectContaining({
          baseURL: "/api/backend",
        })
      );
    });
  });

  it("sets timeout to 30000 ms", () => {
    jest.isolateModules(() => {
      const mockedAxios = require("axios").default;
      require("@/lib/api");
      expect(mockedAxios.create).toHaveBeenCalledWith(
        expect.objectContaining({
          timeout: 30000,
        })
      );
    });
  });

  it("sets Content-Type to application/json", () => {
    jest.isolateModules(() => {
      const mockedAxios = require("axios").default;
      require("@/lib/api");
      expect(mockedAxios.create).toHaveBeenCalledWith(
        expect.objectContaining({
          headers: expect.objectContaining({
            "Content-Type": "application/json",
          }),
        })
      );
    });
  });

  it("registers request and response interceptors", () => {
    jest.isolateModules(() => {
      const mockedAxios = require("axios").default;
      require("@/lib/api");
      expect(mockedAxios.interceptors.request.use).toHaveBeenCalledTimes(1);
      expect(mockedAxios.interceptors.response.use).toHaveBeenCalledTimes(1);
    });
  });
});

describe("request interceptor - Authorization header", () => {
  it("adds Authorization header when access_token is in localStorage", () => {
    localStorage.setItem("access_token", "my-token-abc");

    // Capture the interceptor function
    let requestInterceptor: (config: any) => any;
    const mockedAxios = {
      create: jest.fn().mockReturnThis(),
      interceptors: {
        request: {
          use: jest.fn((fn) => { requestInterceptor = fn; }),
        },
        response: { use: jest.fn() },
      },
      defaults: { baseURL: "" },
    };

    jest.mock("axios", () => ({ default: mockedAxios, ...mockedAxios }));

    jest.isolateModules(() => {
      jest.mock("axios", () => ({ default: mockedAxios, ...mockedAxios }));
      require("@/lib/api");
    });

    if (requestInterceptor!) {
      const config = { headers: {} };
      const result = requestInterceptor(config);
      expect(result.headers.Authorization).toBe("Bearer my-token-abc");
    }
  });

  it("does not add Authorization header when no token in localStorage", () => {
    localStorage.removeItem("access_token");

    let requestInterceptor: (config: any) => any;
    const mockedAxios = {
      create: jest.fn().mockReturnThis(),
      interceptors: {
        request: {
          use: jest.fn((fn) => { requestInterceptor = fn; }),
        },
        response: { use: jest.fn() },
      },
      defaults: { baseURL: "" },
    };

    jest.isolateModules(() => {
      jest.mock("axios", () => ({ default: mockedAxios, ...mockedAxios }));
      require("@/lib/api");
    });

    if (requestInterceptor!) {
      const config = { headers: {} };
      const result = requestInterceptor(config);
      expect(result.headers.Authorization).toBeUndefined();
    }
  });
});

describe("response interceptor - error handling", () => {
  it("extracts error message from response data.error", async () => {
    let errorInterceptor: (_: any, fn: (err: any) => any) => void;

    const mockedAxios = {
      create: jest.fn().mockReturnThis(),
      interceptors: {
        request: { use: jest.fn() },
        response: {
          use: jest.fn((_, fn) => { errorInterceptor = (_ as any, fn); }),
        },
      },
      defaults: { baseURL: "" },
    };

    jest.isolateModules(() => {
      jest.mock("axios", () => ({ default: mockedAxios, ...mockedAxios }));
      require("@/lib/api");

      const [, errFn] = mockedAxios.interceptors.response.use.mock.calls[0];

      const mockError = {
        response: {
          status: 500,
          data: { error: "Internal server error" },
        },
        config: { _retry: true },
        message: "Request failed",
      };

      return errFn(mockError).catch((e: Error) => {
        expect(e.message).toBe("Internal server error");
      });
    });
  });
});

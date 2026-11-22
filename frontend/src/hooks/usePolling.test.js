import { renderHook, act } from "@testing-library/react";
import { usePolling } from "./usePolling";

describe("usePolling", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  test("calls callback immediately and on intervals", () => {
    const callback = jest.fn();
    renderHook(() => usePolling(callback, 5000));

    expect(callback).toHaveBeenCalledTimes(1);

    act(() => {
      jest.advanceTimersByTime(5000);
    });
    expect(callback).toHaveBeenCalledTimes(2);

    act(() => {
      jest.advanceTimersByTime(5000);
    });
    expect(callback).toHaveBeenCalledTimes(3);
  });

  test("does not poll when enabled is false", () => {
    const callback = jest.fn();
    renderHook(() => usePolling(callback, 5000, false));

    expect(callback).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(10000);
    });
    expect(callback).not.toHaveBeenCalled();
  });

  test("cleans up interval on unmount", () => {
    const callback = jest.fn();
    const { unmount } = renderHook(() => usePolling(callback, 5000));

    expect(callback).toHaveBeenCalledTimes(1);
    unmount();

    act(() => {
      jest.advanceTimersByTime(15000);
    });
    expect(callback).toHaveBeenCalledTimes(1);
  });
});

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useMobileDetect, getMobileGraphConfig } from './useMobileDetect';

describe('useMobileDetect', () => {
    let originalInnerWidth: number;
    let originalNavigator: Navigator;
    let originalDevicePixelRatio: number;
    let originalOntouchstart: unknown;

    beforeEach(() => {
        originalInnerWidth = window.innerWidth;
        originalNavigator = window.navigator;
        originalDevicePixelRatio = window.devicePixelRatio;
        originalOntouchstart = (window as unknown as { ontouchstart?: unknown }).ontouchstart;
    });

    afterEach(() => {
        // Restore original values
        Object.defineProperty(window, 'innerWidth', {
            writable: true,
            configurable: true,
            value: originalInnerWidth,
        });
        Object.defineProperty(window, 'navigator', {
            writable: true,
            configurable: true,
            value: originalNavigator,
        });
        Object.defineProperty(window, 'devicePixelRatio', {
            writable: true,
            configurable: true,
            value: originalDevicePixelRatio,
        });
        if (originalOntouchstart === undefined) {
            delete (window as unknown as { ontouchstart?: unknown }).ontouchstart;
        } else {
            (window as unknown as { ontouchstart?: unknown }).ontouchstart = originalOntouchstart;
        }
    });

    it('detects mobile device with width < 768px', () => {
        Object.defineProperty(window, 'innerWidth', {
            writable: true,
            configurable: true,
            value: 375,
        });

        const { result } = renderHook(() => useMobileDetect());

        expect(result.current.isMobile).toBe(true);
        expect(result.current.isTablet).toBe(false);
        expect(result.current.screenWidth).toBe(375);
    });

    it('detects tablet device with width 768-1024px', () => {
        Object.defineProperty(window, 'innerWidth', {
            writable: true,
            configurable: true,
            value: 800,
        });

        const { result } = renderHook(() => useMobileDetect());

        expect(result.current.isMobile).toBe(false);
        expect(result.current.isTablet).toBe(true);
        expect(result.current.screenWidth).toBe(800);
    });

    it('detects desktop device with width >= 1024px', () => {
        Object.defineProperty(window, 'innerWidth', {
            writable: true,
            configurable: true,
            value: 1920,
        });

        const { result } = renderHook(() => useMobileDetect());

        expect(result.current.isMobile).toBe(false);
        expect(result.current.isTablet).toBe(false);
        expect(result.current.screenWidth).toBe(1920);
    });

    it('detects touch capability', () => {
        Object.defineProperty(window, 'ontouchstart', {
            writable: true,
            configurable: true,
            value: () => {},
        });

        const { result } = renderHook(() => useMobileDetect());

        expect(result.current.isTouchDevice).toBe(true);
    });

    it('detects touch via maxTouchPoints', () => {
        const navigatorWithTouch = {
            ...originalNavigator,
            maxTouchPoints: 5,
        };
        
        Object.defineProperty(window, 'navigator', {
            writable: true,
            configurable: true,
            value: navigatorWithTouch,
        });

        const { result } = renderHook(() => useMobileDetect());

        expect(result.current.isTouchDevice).toBe(true);
    });

    it('updates on window resize', () => {
        Object.defineProperty(window, 'innerWidth', {
            writable: true,
            configurable: true,
            value: 1920,
        });

        const { result } = renderHook(() => useMobileDetect());

        expect(result.current.isMobile).toBe(false);
        expect(result.current.screenWidth).toBe(1920);

        // Simulate resize to mobile
        act(() => {
            Object.defineProperty(window, 'innerWidth', {
                writable: true,
                configurable: true,
                value: 375,
            });
            window.dispatchEvent(new Event('resize'));
        });

        expect(result.current.isMobile).toBe(true);
        expect(result.current.screenWidth).toBe(375);
    });

    it('cleans up resize listener on unmount', () => {
        const removeEventListenerSpy = vi.spyOn(window, 'removeEventListener');

        const { unmount } = renderHook(() => useMobileDetect());

        unmount();

        expect(removeEventListenerSpy).toHaveBeenCalledWith(
            'resize',
            expect.any(Function)
        );

        removeEventListenerSpy.mockRestore();
    });
});

describe('getMobileGraphConfig', () => {
    it('returns mobile config for mobile devices', () => {
        const config = getMobileGraphConfig(true, false);

        expect(config.maxRenderNodes).toBe(5000);
        expect(config.maxRenderLinks).toBe(10000);
        expect(config.defaultLODTier).toBe(2); // MEDIUM
        expect(config.pixelRatio).toBeLessThanOrEqual(1.5);
    });

    it('returns tablet config for tablet devices', () => {
        const config = getMobileGraphConfig(false, true);

        expect(config.maxRenderNodes).toBe(10000);
        expect(config.maxRenderLinks).toBe(25000);
        expect(config.defaultLODTier).toBe(2); // MEDIUM
    });

    it('returns desktop config for desktop devices', () => {
        const config = getMobileGraphConfig(false, false);

        expect(config.maxRenderNodes).toBe(20000);
        expect(config.maxRenderLinks).toBe(50000);
        expect(config.defaultLODTier).toBe(3); // HIGH
    });

    it('caps pixel ratio appropriately for mobile', () => {
        Object.defineProperty(window, 'devicePixelRatio', {
            writable: true,
            configurable: true,
            value: 3,
        });

        const mobileConfig = getMobileGraphConfig(true, false);
        expect(mobileConfig.pixelRatio).toBe(1.5); // Capped

        const tabletConfig = getMobileGraphConfig(false, true);
        expect(tabletConfig.pixelRatio).toBe(2); // Capped

        const desktopConfig = getMobileGraphConfig(false, false);
        expect(desktopConfig.pixelRatio).toBe(2); // Capped
    });
});

import { stripRouterKey } from './routerKey';

describe('stripRouterKey', () => {
  it('removes the router key when it is the only param', () => {
    expect(stripRouterKey('/liked?__EXPO_ROUTER_key=undefined-bQ6BGCUf90')).toBe('/liked');
  });

  it('keeps other params and the hash', () => {
    expect(stripRouterKey('/album/123?foo=1&__EXPO_ROUTER_key=x')).toBe('/album/123?foo=1');
    expect(stripRouterKey('/y?__EXPO_ROUTER_key=z#frag')).toBe('/y#frag');
  });

  it('leaves untouched URLs alone', () => {
    expect(stripRouterKey('/plain')).toBe('/plain');
    expect(stripRouterKey('/x?a=1')).toBe('/x?a=1');
  });
});

import { api } from './duoyu'
export function fetchGetUserInfo(){return api<Api.Auth.UserInfo>('/api/user/info')}
export async function fetchLogin(_params:Api.Auth.LoginParams):Promise<Api.Auth.LoginResponse>{window.location.assign('/login');throw new Error('请前往统一登录页面')}
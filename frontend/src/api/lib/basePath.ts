import axios from "axios";

export const GATEWAY_BASE_URL = 'http://localhost:8080'
export const GatewayApi = axios.create({baseURL: GATEWAY_BASE_URL});
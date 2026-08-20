import axios from "axios";

export const GatewayApi = axios.create({baseURL: 'http://localhost:8080'});
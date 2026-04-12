import axios from "axios";

export const VideoApi = axios.create({baseURL: 'http://localhost:8080'});

export const StatusApi = axios.create({baseURL: 'http://localhost:8081'});
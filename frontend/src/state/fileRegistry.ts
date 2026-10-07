/**Module-scope so component unmount can't drop in-flight files or abort handles*/
export const fileMap = new Map<number, File>()
export const abortRefs = new Map<number, () => void>()

let nextId = 0
export const nextFileId = () => nextId++
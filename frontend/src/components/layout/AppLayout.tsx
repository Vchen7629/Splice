import type { ReactNode } from "react";

interface AppLayoutProps {
    header: ReactNode
    sidebar: ReactNode
    children: ReactNode
}

/** Layout for the app page (header bar, main zone, right sidebar) */
const AppLayout = ({ header, sidebar, children }: AppLayoutProps) => (
    <div className="flex flex-col h-[100svh] bg-bg">
        {header}
        <div className="flex flex-1 min-h-0">
            {children}
            {sidebar}
        </div>
    </div>
)

export default AppLayout
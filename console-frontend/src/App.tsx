import { useEffect, useState } from "react";
import "./App.css";
import { OriginalConsole } from "./OriginalConsole";

const VIEW_STORAGE_KEY = "gwf-console-view";

type ViewType = "console" | "alert";

const resolveInitialView = (): ViewType => {
  if (typeof window === "undefined") return "console";
  const stored = window.localStorage.getItem(VIEW_STORAGE_KEY);
  if (stored === "alert") return "alert";
  return "console";
};

function App() {
  const [view, setView] = useState<ViewType>(() => resolveInitialView());

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(VIEW_STORAGE_KEY, view);
  }, [view]);

  return <OriginalConsole view={view} onViewChange={setView} />;
}

export default App;

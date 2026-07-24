import { BrowserRouter } from "react-router-dom";
import { Toaster } from "sonner";
import { AuthProvider } from "./contexts/AuthContext";
import AuthenticatedLayout from "./routes/AuthenticatedLayout";

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AuthenticatedLayout />
      </AuthProvider>
      <Toaster theme="dark" position="bottom-right" />
    </BrowserRouter>
  );
}

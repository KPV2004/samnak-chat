import { Navigate, Route, Routes } from "react-router";
import { Home } from "./Home";
import { AcceptInvite } from "./pages/AcceptInvite";
import { Devices } from "./pages/Devices";
import { Invites } from "./pages/Invites";
import { Login } from "./pages/Login";
import { Members } from "./pages/Members";
import { NewWorkspace } from "./pages/NewWorkspace";
import { Profile } from "./pages/Profile";
import { Register } from "./pages/Register";
import { Shell } from "./pages/Shell";

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/register" element={<Register />} />
      <Route element={<Shell />}>
        <Route index element={<Home />} />
        <Route path="members" element={<Members />} />
        <Route path="invites" element={<Invites />} />
        <Route path="profile" element={<Profile />} />
        <Route path="devices" element={<Devices />} />
        <Route path="workspaces/new" element={<NewWorkspace />} />
        <Route path="invite/:token" element={<AcceptInvite />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

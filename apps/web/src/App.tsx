import { Navigate, Route, Routes } from "react-router";
import { ChannelPage } from "./chat/ChannelPage";
import { BrowseChannels } from "./chat/BrowseChannels";
import { ChatHome, ChatLayout } from "./chat/ChatLayout";
import { NewChannel } from "./chat/NewChannel";
import { NewDm } from "./chat/NewDm";
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
import "./approval/register";
import { ApprovalDetail } from "./approval/ApprovalDetail";
import { Approvals } from "./approval/Approvals";
import { RequireModule } from "./approval/useModule";

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/register" element={<Register />} />
      <Route element={<Shell />}>
        <Route index element={<Home />} />
        <Route path="chat" element={<ChatLayout />}>
          <Route index element={<ChatHome />} />
          <Route path="browse" element={<BrowseChannels />} />
          <Route path="new" element={<NewChannel />} />
          <Route path="dm" element={<NewDm />} />
          <Route path=":channelId" element={<ChannelPage />} />
        </Route>
        <Route path="approvals" element={<RequireModule name="approval"><Approvals /></RequireModule>} />
        <Route path="approvals/:requestId" element={<RequireModule name="approval"><ApprovalDetail /></RequireModule>} />
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

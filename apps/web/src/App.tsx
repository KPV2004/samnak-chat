import { lazy, type ComponentType } from "react";
import { Navigate, Route, Routes } from "react-router";
import { Login } from "./pages/Login";
import { Register } from "./pages/Register";
import { Shell } from "./pages/Shell";
import { RequireModule } from "./modules";
import "./approval/register";

/**
 * Pages load on first visit, so the first screen carries only the shell and the sign-in pages.
 * `Shell` shows a skeleton while a page loads (see `RouteFallback`).
 */
function page<K extends string>(load: () => Promise<Record<K, ComponentType>>, name: K) {
  return lazy(() => load().then((m) => ({ default: m[name] })));
}

const Home = page(() => import("./Home"), "Home");
const ChatLayout = page(() => import("./chat/ChatLayout"), "ChatLayout");
const ChatHome = page(() => import("./chat/ChatLayout"), "ChatHome");
const BrowseChannels = page(() => import("./chat/BrowseChannels"), "BrowseChannels");
const NewChannel = page(() => import("./chat/NewChannel"), "NewChannel");
const NewDm = page(() => import("./chat/NewDm"), "NewDm");
const ChannelPage = page(() => import("./chat/ChannelPage"), "ChannelPage");
const MyTasks = page(() => import("./task/MyTasks"), "MyTasks");
const Approvals = page(() => import("./approval/Approvals"), "Approvals");
const ApprovalDetail = page(() => import("./approval/ApprovalDetail"), "ApprovalDetail");
const Groups = page(() => import("./groups/Groups"), "Groups");
const Members = page(() => import("./pages/Members"), "Members");
const Invites = page(() => import("./pages/Invites"), "Invites");
const Profile = page(() => import("./pages/Profile"), "Profile");
const Devices = page(() => import("./pages/Devices"), "Devices");
const NewWorkspace = page(() => import("./pages/NewWorkspace"), "NewWorkspace");
const AcceptInvite = page(() => import("./pages/AcceptInvite"), "AcceptInvite");

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
        <Route path="tasks" element={<MyTasks />} />
        <Route path="approvals" element={<RequireModule name="approval"><Approvals /></RequireModule>} />
        <Route path="approvals/:requestId" element={<RequireModule name="approval"><ApprovalDetail /></RequireModule>} />
        <Route path="groups" element={<RequireModule name="approval"><Groups /></RequireModule>} />
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
